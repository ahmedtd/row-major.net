// Package scraper houses the logic for determining which stories are
// of interest.
package scraper

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"row-major/rumor-mill/hackernews"
	trackerpb "row-major/rumor-mill/scraper/trackerpb"

	"cloud.google.com/go/storage"
	"github.com/golang/glog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/proto"
)

type trackerState int

func hnURL(t *trackerpb.TrackedArticle) string {
	return fmt.Sprintf("https://news.ycombinator.com/item?id=%d", t.GetId())
}

type hnClient interface {
	TopStories(context.Context) ([]uint64, error)
	Item(context.Context, uint64) (*hackernews.Item, error)
	Items(context.Context, []uint64) ([]*hackernews.Item, error)
}

const trackedArticleKeyPrefix = "hackernews-tracked-articles/"

type TrackedArticleTable struct {
	gcs    *storage.Client
	bucket string
}

func NewTrackedArticleTable(gcs *storage.Client, bucket string) *TrackedArticleTable {
	return &TrackedArticleTable{
		gcs:    gcs,
		bucket: bucket,
	}
}

func (t *TrackedArticleTable) gcsPathForID(id uint64) string {
	return path.Join(trackedArticleKeyPrefix, strconv.FormatUint(id, 10))
}

func (t *TrackedArticleTable) idFromGCSName(name string) (uint64, error) {
	return strconv.ParseUint(strings.TrimPrefix(name, trackedArticleKeyPrefix), 10, 64)
}

// Get gets the TrackedArticle with the given ID from GCS.
//
// Returns the TrackedArticle, a "found" indicator, and an error.
func (t *TrackedArticleTable) Get(ctx context.Context, id uint64) (*trackerpb.TrackedArticle, bool, error) {
	tracer := otel.Tracer("row-major/rumor-mill/scraper")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "TrackedArticleTable.Get")
	defer span.End()

	span.SetAttributes(attribute.Int64("id", int64(id)))

	obj := t.gcs.Bucket(t.bucket).Object(t.gcsPathForID(id))

	r, err := obj.NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			span.SetStatus(codes.Ok, "")
			return nil, false, nil
		}

		err := fmt.Errorf("while opening reader for object: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, false, err
	}
	defer r.Close()

	data, err := ioutil.ReadAll(r)
	if err != nil {
		err := fmt.Errorf("while reading from object: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, false, err
	}

	ta := &trackerpb.TrackedArticle{}
	if err := proto.Unmarshal(data, ta); err != nil {
		err := fmt.Errorf("while unmarshaling TrackedArticle proto: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, false, err
	}

	if id != ta.Id {
		err := fmt.Errorf("ID mismatch in TrackedArticle")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, false, err
	}

	ta.Generation = r.Attrs.Generation
	ta.Metageneration = r.Attrs.Metageneration

	span.SetStatus(codes.Ok, "")

	return ta, true, nil
}

func (t *TrackedArticleTable) Create(ctx context.Context, ta *trackerpb.TrackedArticle) error {
	tracer := otel.Tracer("row-major/rumor-mill/scraper")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "TrackedArticleTable.Create")
	defer span.End()

	obj := t.gcs.Bucket(t.bucket).Object(t.gcsPathForID(ta.Id))

	// Make sure that the GCS-specific metadata is zeroed out before writing the
	// object to storage.
	savedGeneration := ta.Generation
	savedMetageneration := ta.Metageneration
	ta.Generation = 0
	ta.Metageneration = 0
	defer func() {
		ta.Generation = savedGeneration
		ta.Metageneration = savedMetageneration
	}()

	data, err := proto.Marshal(ta)
	if err != nil {
		return fmt.Errorf("while marshaling TrackedArticle proto: %w", err)
	}

	// Create condition: object does not currently exist.
	w := obj.If(storage.Conditions{DoesNotExist: true}).NewWriter(ctx)

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("while writing TrackedArticle to object writer: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("while closing object writer: %w", err)
	}

	return nil
}

func (t *TrackedArticleTable) Update(ctx context.Context, ta *trackerpb.TrackedArticle) error {
	tracer := otel.Tracer("row-major/rumor-mill/scraper")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "TrackedArticleTable.Update")
	defer span.End()

	obj := t.gcs.Bucket(t.bucket).Object(t.gcsPathForID(ta.Id))

	// Make sure that the GCS-specific metadata is zeroed out before writing the
	// object back to storage.
	savedGeneration := ta.Generation
	savedMetageneration := ta.Metageneration
	ta.Generation = 0
	ta.Metageneration = 0
	defer func() {
		ta.Generation = savedGeneration
		ta.Metageneration = savedMetageneration
	}()

	data, err := proto.Marshal(ta)
	if err != nil {
		return fmt.Errorf("while marshaling TrackedArticle proto: %w", err)
	}

	// Update condition: object exists at the generation we're working from.
	w := obj.If(storage.Conditions{GenerationMatch: savedGeneration}).NewWriter(ctx)

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("while writing TrackedArticle to object writer: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("while closing object writer: %w", err)
	}

	return nil
}

type TrackedArticleIterator struct {
	table *TrackedArticleTable
	inner *storage.ObjectIterator
}

func (it *TrackedArticleIterator) Next(ctx context.Context) (*trackerpb.TrackedArticle, error) {
	tracer := otel.Tracer("row-major/rumor-mill/scraper")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "TrackedArticleIterator.Next")
	defer span.End()

	for {
		attr, err := it.inner.Next()
		if err != nil {
			return nil, err
		}

		id, err := it.table.idFromGCSName(attr.Name)
		if err != nil {
			return nil, fmt.Errorf("while parsing ID: %w", err)
		}

		ta, ok, err := it.table.Get(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("while reading tracked article: %w", err)
		}

		if !ok {
			// Object was deleted during list.
			continue
		}

		return ta, nil
	}
}

func (t *TrackedArticleTable) List(ctx context.Context) *TrackedArticleIterator {
	return &TrackedArticleIterator{
		table: t,
		inner: t.gcs.Bucket(t.bucket).Objects(ctx, &storage.Query{Prefix: trackedArticleKeyPrefix}),
	}
}

// Scraper checks data sources for articles matching the specified topic regexp.
type Scraper struct {
	hn hnClient

	trackedArticles *TrackedArticleTable

	watchConfigs map[uint64]*WatchConfig
}

type ScraperOpt func(*Scraper)

// WatchConfig binds together a set of data source configurations and a set of
// notification targets.
type WatchConfig struct {
	ID              uint64
	TopicRegexp     *regexp.Regexp
	NotifyAddresses []string
}

func WithWatchConfig(wc *WatchConfig) ScraperOpt {
	return func(s *Scraper) {
		s.watchConfigs[wc.ID] = wc
	}
}

// New creates a new Scraper
func New(hn hnClient, trackedArticles *TrackedArticleTable, opts ...ScraperOpt) *Scraper {
	scraper := &Scraper{
		hn:              hn,
		trackedArticles: trackedArticles,
		watchConfigs:    map[uint64]*WatchConfig{},
	}

	for _, opt := range opts {
		opt(scraper)
	}

	// TODO: Validate watchConfigs (unique IDs).

	return scraper
}

// Run starts the Scraper's loop.
func (s *Scraper) Run(ctx context.Context) {

	// Scrape right away
	if err := s.scraperPass(ctx); err != nil {
		glog.Errorf("Error while running scraper pass: %v", err)
	}

	ticker := time.NewTicker(5 * time.Minute)
	for {
		select {
		case <-ctx.Done():
			glog.Infof("Shutting down scraper")
			return
		case <-ticker.C:
		}
		if err := s.scraperPass(ctx); err != nil {
			glog.Errorf("Error while running scraper pass: %v", err)
		}
	}
}

func (s *Scraper) scraperPass(ctx context.Context) error {
	tracer := otel.Tracer("row-major/rumor-mill/scraper")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "Scraper.scraperPass")
	defer span.End()

	if err := s.ingestTopStories(ctx); err != nil {
		return fmt.Errorf("while scraping: %w", err)
	}

	if err := s.sendAlerts(ctx); err != nil {
		return fmt.Errorf("while sending alerts: %w", err)
	}

	return nil
}

func (s *Scraper) ingestTopStories(ctx context.Context) error {
	tracer := otel.Tracer("row-major/rumor-mill/scraper")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "Scraper.ingestTopStories")
	defer span.End()

	topStories, err := s.hn.TopStories(ctx)
	if err != nil {
		return fmt.Errorf("while querying for top stories: %w", err)
	}

	// TODO(ahmedtd): Retry loop for conflicts on each article

	for rank, id := range topStories {
		if err := s.ingestTopStory(ctx, rank, id); err != nil {
			return fmt.Errorf("while ingesting top story id=%d rank=%d: %w", id, rank, err)
		}
	}

	return nil
}

func (s *Scraper) ingestTopStory(ctx context.Context, rank int, id uint64) error {
	now := time.Now().UnixNano()

	ta, ok, err := s.trackedArticles.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("while loading TrackedArticle id=%d: %w", id, err)
	}
	if !ok {
		item, err := s.hn.Item(ctx, id)
		if err != nil {
			return fmt.Errorf("while fetching item %d from HN: %w", id, err)
		}

		ta = &trackerpb.TrackedArticle{
			Id:             id,
			FirstSeenTime:  now,
			LatestSeenTime: now,
			LatestRank:     int64(rank) + 1,
			Title:          item.Title,
			Submitter:      item.By,
		}

		if err := s.trackedArticles.Create(ctx, ta); err != nil {
			return fmt.Errorf("while creating tracked article: %w", err)
		}

		return nil
	}

	ta.LatestSeenTime = now
	ta.LatestRank = int64(rank) + 1

	if err := s.trackedArticles.Update(ctx, ta); err != nil {
		return fmt.Errorf("while updating tracked article: %w", err)
	}

	return nil
}

func (s *Scraper) sendAlerts(ctx context.Context) error {
	tracer := otel.Tracer("row-major/rumor-mill/scraper")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "Scraper.sendAlerts")
	defer span.End()

	it := s.trackedArticles.List(ctx)
	for {
		ta, err := it.Next(ctx)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("while advancing article iterator: %w", err)
		}

		for _, wc := range s.watchConfigs {
			if ta.LatestRank < 500 && wc.TopicRegexp.MatchString(strings.ToLower(ta.Title)) {
				// Send alert

				// Record that this watchconfig has alerted.
			}
		}
	}

	return nil
}

const articlesHTML = `
<!DOCTYPE html>
<head>
	<title>HN Article State</title>
</head>

<h1>Interested Articles</h1>
<ul>
{{range .InterestedArticles}}
<li>({{.LatestRank}}) <a href="{{.URL}}">{{.Title}}</a>; submitted by {{.Submitter}}</li>
{{end}}
</ul>

<h1>Not Interested Articles</h1>
<ul>
{{range .NotInterestedArticles}}
<li>({{.LatestRank}}) <a href="{{.URL}}">{{.Title}}</a>; submitted by {{.Submitter}}</li>
{{end}}
</ul>

`

var articlesTemplate = template.Must(template.New("articles").Parse(articlesHTML))

func (s *Scraper) RegisterDebugHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/rumor_mill/articles", s.debugHandlerArticles)
}

func (s *Scraper) debugHandlerArticles(w http.ResponseWriter, req *http.Request) {
	tracer := otel.Tracer("row-major/rumor-mill/scraper")
	ctx, span := tracer.Start(req.Context(), "Scraper.DebugHandlerArticles")
	defer span.End()

	type TmplArticle struct {
		LatestRank int64
		Title      string
		Submitter  string
		URL        string
	}
	type TmplData struct {
		InterestedArticles    []TmplArticle
		NotInterestedArticles []TmplArticle
	}
	tmplData := TmplData{}

	it := s.trackedArticles.List(ctx)
	for {
		ta, err := it.Next(ctx)
		if err == iterator.Done {
			break
		}
		if err != nil {
			glog.Errorf("Error while advancing article iterator: %v", err)
			http.Error(w, "Failed to list articles", http.StatusInternalServerError)
			return
		}

		tmplArticle := TmplArticle{
			LatestRank: ta.LatestRank,
			Title:      ta.Title,
			Submitter:  ta.Submitter,
			URL:        hnURL(ta),
		}

		if len(ta.FiredWatchConfigs) != 0 {
			tmplData.InterestedArticles = append(tmplData.InterestedArticles, tmplArticle)
		} else {
			tmplData.NotInterestedArticles = append(tmplData.NotInterestedArticles, tmplArticle)
		}

		sort.Slice(tmplData.InterestedArticles, func(i, j int) bool {
			return tmplData.InterestedArticles[i].LatestRank < tmplData.InterestedArticles[j].LatestRank
		})
		sort.Slice(tmplData.NotInterestedArticles, func(i, j int) bool {
			return tmplData.NotInterestedArticles[i].LatestRank < tmplData.NotInterestedArticles[j].LatestRank
		})
	}

	articlesTemplate.Execute(w, tmplData)
}
