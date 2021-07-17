// Package scraper houses the logic for determining which stories are
// of interest.
package scraper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	texttemplate "text/template"
	"time"

	"row-major/rumor-mill/hackernews"
	"row-major/rumor-mill/table"
	trackerpb "row-major/rumor-mill/table/trackerpb"

	"github.com/golang/glog"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/encoding/prototext"
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

// Scraper checks data sources for articles matching the specified topic regexp.
type Scraper struct {
	hn hnClient
	sg *sendgrid.Client

	trackedArticles *table.TrackedArticleTable

	watchConfigs map[uint64]*WatchConfig

	scrapePeriod time.Duration
}

type ScraperOpt func(*Scraper)

// WatchConfig binds together a set of data source configurations and a set of
// notification targets.
type WatchConfig struct {
	ID              uint64
	Description     string
	TopicRegexp     *regexp.Regexp
	NotifyAddresses []string
}

func WithWatchConfig(wc *WatchConfig) ScraperOpt {
	return func(s *Scraper) {
		s.watchConfigs[wc.ID] = wc
	}
}

func WithScrapePeriod(period time.Duration) ScraperOpt {
	return func(s *Scraper) {
		s.scrapePeriod = period
	}
}

// New creates a new Scraper
func New(hn hnClient, sg *sendgrid.Client, trackedArticles *table.TrackedArticleTable, opts ...ScraperOpt) *Scraper {
	scraper := &Scraper{
		hn:              hn,
		sg:              sg,
		trackedArticles: trackedArticles,
		watchConfigs:    map[uint64]*WatchConfig{},
		scrapePeriod:    30 * time.Minute,
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

	ticker := time.NewTicker(s.scrapePeriod)
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

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	if err := s.ingestTopStories(ctx); err != nil {
		return fmt.Errorf("while scraping: %w", err)
	}

	if err := s.sweepOldStories(ctx); err != nil {
		return fmt.Errorf("while sweeping old stories: %w", err)
	}

	if err := s.sendAlerts(ctx); err != nil {
		return fmt.Errorf("while sending alerts: %w", err)
	}

	glog.Infof("Successfully completed scraper pass")

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

	// Use errgroup and semaphore to limit concurrency.
	eg, ctx := errgroup.WithContext(ctx)
	sem := semaphore.NewWeighted(500)

	for rank, id := range topStories {
		rank, id := rank, id // https://golang.org/doc/faq#closures_and_goroutines

		if err := sem.Acquire(ctx, 1); err != nil {
			return fmt.Errorf("while acquiring concurrency limiter semaphore: %w", err)
		}

		eg.Go(func() error {
			defer sem.Release(1)
			if err := s.ingestTopStory(ctx, rank, id); err != nil {
				return fmt.Errorf("while ingesting top story id=%d rank=%d: %w", id, rank, err)
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("while waiting for completion of errgroup: %w", err)
	}

	return nil
}

func (s *Scraper) ingestTopStory(ctx context.Context, rank int, id uint64) error {
	tracer := otel.Tracer("row-major/rumor-mill/scraper")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "Scraper.ingestTopStory")
	defer span.End()

readModifyWrite:
	for {
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
			var gErr *googleapi.Error
			if errors.As(err, &gErr) {
				if gErr.Code == 412 {
					// Bad precondition, retry.
					continue readModifyWrite
					// TODO(ahmedtd): This retry loop ends up clobbering with
					// stale rank.  To be correct, we need to re-read current
					// rank.
				}
			}
			return fmt.Errorf("while updating tracked article: %w", err)
		}

		return nil
	}
}

// sweepOldStories does a table scan of the tracked articles table, moving older
// stories to the stale tracked articles table.
func (s *Scraper) sweepOldStories(ctx context.Context) error {
	tracer := otel.Tracer("row-major/rumor-mill/scraper")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "Scraper.sweepOldStories")
	defer span.End()

	// Use errgroup and semaphore to limit concurrency.
	eg, ctx := errgroup.WithContext(ctx)
	sem := semaphore.NewWeighted(500)

	it := s.trackedArticles.ListIDs(ctx)
	for {
		id, err := it.Next(ctx)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("while advancing tracked article ID iterator: %w", err)
		}

		if err := sem.Acquire(ctx, 1); err != nil {
			return fmt.Errorf("while acquiring concurrency limiter semaphore: %w", err)
		}

		eg.Go(func() error {
			defer sem.Release(1)
			if err := s.sweepOldStory(ctx, id); err != nil {
				return fmt.Errorf("while sweeping story story id=%d: %w", id, err)
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("while waiting for completion of errgroup: %w", err)
	}

	return nil
}

func (s *Scraper) sweepOldStory(ctx context.Context, id uint64) error {
	tracer := otel.Tracer("row-major/rumor-mill/scraper")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "Scraper.sweepOldStory")
	defer span.End()

readModifyWrite:
	for {
		ta, ok, err := s.trackedArticles.Get(ctx, id)
		if err != nil {
			return fmt.Errorf("while loading TrackedArticle id=%d: %w", id, err)
		}
		if !ok {
			// Article removed during list.
			return nil
		}

		// If article appeared within the top 500 articles in the last 30
		// minutes, leave it alone.
		if time.Now().Sub(time.Unix(0, ta.GetLatestSeenTime())) <= 30*time.Minute {
			return nil
		}

		// Article is old.  Delete it.
		//
		// TODO(ahmedtd): Sweep to stale tracked article table.
		if err := s.trackedArticles.Delete(ctx, ta); err != nil {
			var gErr *googleapi.Error
			if errors.As(err, &gErr) {
				if gErr.Code == 412 {
					// Bad precondition, retry.
					continue readModifyWrite
				}
			}
			return fmt.Errorf("while deleting original tracked article: %w", err)
		}
	}
}

func (s *Scraper) sendAlerts(ctx context.Context) error {
	tracer := otel.Tracer("row-major/rumor-mill/scraper")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "Scraper.sendAlerts")
	defer span.End()

	// Use errgroup and semaphore to limit concurrency.
	eg, ctx := errgroup.WithContext(ctx)
	sem := semaphore.NewWeighted(500)

	it := s.trackedArticles.ListIDs(ctx)
	for {
		id, err := it.Next(ctx)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("while advancing article iterator: %w", err)
		}

		if err := sem.Acquire(ctx, 1); err != nil {
			return fmt.Errorf("while acquiring concurrency limiter semaphore: %w", err)
		}

		eg.Go(func() error {
			defer sem.Release(1)

			if err := s.sendAlertsForArticle(ctx, id); err != nil {
				return fmt.Errorf("while sending alerts for article id=%d: %w", id, err)
			}

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("while waiting for completion of errgroup: %w", err)
	}

	return nil
}

func (s *Scraper) sendAlertsForArticle(ctx context.Context, id uint64) error {
	tracer := otel.Tracer("row-major/rumor-mill/scraper")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "Scraper.sendAlertsForArticle")
	defer span.End()

readModifyWrite:
	for {
		ta, ok, err := s.trackedArticles.Get(ctx, id)
		if err != nil {
			return fmt.Errorf("while fetching article id=%d: %w", id, err)
		}
		if !ok {
			// Article deleted during scan.
			return nil
		}

		for _, wc := range s.watchConfigs {
			if err := s.sendAlertForArticleAndWatchConfig(ctx, ta, wc); err != nil {
				return fmt.Errorf("while checking article %d against watchconfig %d: %w", id, wc.ID, err)
			}
		}

		if err := s.trackedArticles.Update(ctx, ta); err != nil {
			var gErr *googleapi.Error
			if errors.As(err, &gErr) {
				if gErr.Code == 412 {
					// Bad precondition, retry.
					continue readModifyWrite
				}
			}
			return fmt.Errorf("while updating tracked article: %w", err)
		}

		return nil
	}
}

const emailPlain = `There's a new Hacker News article matching your watch config:
* Article: {{.ArticleTitle}}
* Link: {{.ArticleLink}}
* Watch Config: {{.WatchConfigDescription}}
`

var emailPlainTemplate = texttemplate.Must(texttemplate.New("email").Parse(emailPlain))

func (s *Scraper) sendAlertForArticleAndWatchConfig(ctx context.Context, ta *trackerpb.TrackedArticle, wc *WatchConfig) error {
	// Have we already fired an alert for this watch config?
	for _, wcID := range ta.FiredWatchConfigs {
		if wcID == wc.ID {
			return nil
		}
	}

	if ta.LatestRank >= 500 {
		return nil
	}
	if !wc.TopicRegexp.MatchString(strings.ToLower(ta.Title)) {
		return nil
	}

	// The article is relevant to the watchconfig.

	message := mail.NewV3Mail()
	message.From = mail.NewEmail("Rumor Mill Bot", "bot@rumor-mill.dev")
	message.Subject = fmt.Sprintf("New HackerNews Article: %s", ta.Title)

	p := mail.NewPersonalization()
	for _, addr := range wc.NotifyAddresses {
		p.To = append(p.To, mail.NewEmail("", addr))
	}
	message.Personalizations = append(message.Personalizations, p)

	params := &struct {
		ArticleTitle           string
		ArticleLink            string
		WatchConfigDescription string
	}{
		ArticleTitle:           ta.Title,
		ArticleLink:            hnURL(ta),
		WatchConfigDescription: wc.Description,
	}

	textContent := &bytes.Buffer{}
	err := emailPlainTemplate.Execute(textContent, params)
	if err != nil {
		return fmt.Errorf("while templating plain-text email content: %w", err)
	}

	message.Content = append(message.Content, mail.NewContent("text/plain", string(textContent.Bytes())))

	resp, err := s.sg.SendWithContext(ctx, message)
	if err != nil {
		return fmt.Errorf("while sending mail through SendGrid: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("non-2xx response while sending mail through SendGrid: %d %q", resp.StatusCode, resp.Body)
	}

	ta.FiredWatchConfigs = append(ta.FiredWatchConfigs, wc.ID)

	return nil
}

func (s *Scraper) RegisterDebugHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/rumor-mill/tracked-articles", s.debugHandlerTrackedArticles)
	mux.HandleFunc("/rumor-mill/datastore/tracked-article/", s.debugHandlerTrackedArticle)
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

type TrackedArticleData struct {
	LatestRank int64
	Title      string
	Submitter  string
	URL        string
}

type TrackedArticlesData struct {
	InterestedArticles    []TrackedArticleData
	NotInterestedArticles []TrackedArticleData
}

func (s *Scraper) debugHandlerTrackedArticles(w http.ResponseWriter, req *http.Request) {
	tracer := otel.Tracer("row-major/rumor-mill/scraper")
	ctx, span := tracer.Start(req.Context(), "Scraper.debugHandlerTrackedArticles")
	defer span.End()

	tmplData, err := s.debugHandlerTrackedArticlesData(ctx)
	if err != nil {
		glog.Errorf("Error while retrieving tracked article data: %w", err)
		http.Error(w, "error retrieving tracked article data", http.StatusInternalServerError)
		return
	}

	if err := articlesTemplate.Execute(w, tmplData); err != nil {
		glog.Errorf("Error while executing template: %w", err)
		return
	}
}

func (s *Scraper) debugHandlerTrackedArticlesData(ctx context.Context) (*TrackedArticlesData, error) {
	tmplData := &TrackedArticlesData{}
	tmplDataLock := sync.Mutex{}

	// Use errgroup and semaphore to limit concurrency.
	eg, ctx := errgroup.WithContext(ctx)
	sem := semaphore.NewWeighted(500)

	it := s.trackedArticles.ListIDs(ctx)
	for {
		id, err := it.Next(ctx)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("while advancing article iterator: %w", err)
		}

		if err := sem.Acquire(ctx, 1); err != nil {
			return nil, fmt.Errorf("while acquiring concurrency limiter semaphore: %w", err)
		}

		eg.Go(func() error {
			defer sem.Release(1)

			ta, ok, err := s.trackedArticles.Get(ctx, id)
			if err != nil {
				return fmt.Errorf("while getting article id=%d: %w", id, err)
			}
			if !ok {
				return nil
			}

			tmplArticle := TrackedArticleData{
				LatestRank: ta.LatestRank,
				Title:      ta.Title,
				Submitter:  ta.Submitter,
				URL:        hnURL(ta),
			}

			tmplDataLock.Lock()
			defer tmplDataLock.Unlock()
			if len(ta.FiredWatchConfigs) != 0 {
				tmplData.InterestedArticles = append(tmplData.InterestedArticles, tmplArticle)
			} else {
				tmplData.NotInterestedArticles = append(tmplData.NotInterestedArticles, tmplArticle)
			}

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, fmt.Errorf("while waiting for completion of errgroup: %w", err)
	}

	sort.Slice(tmplData.InterestedArticles, func(i, j int) bool {
		return tmplData.InterestedArticles[i].LatestRank < tmplData.InterestedArticles[j].LatestRank
	})
	sort.Slice(tmplData.NotInterestedArticles, func(i, j int) bool {
		return tmplData.NotInterestedArticles[i].LatestRank < tmplData.NotInterestedArticles[j].LatestRank
	})

	return tmplData, nil
}

const trackedArticleHTML = `
<!DOCTYPE html>
<head>
	<title>Tracked Article</title>
</head>

<pre>
{{.TextProto}}
</pre>
`

var trackedArticleTemplate = template.Must(template.New("articles").Parse(trackedArticleHTML))

func (s *Scraper) debugHandlerTrackedArticle(w http.ResponseWriter, req *http.Request) {
	tracer := otel.Tracer("row-major/rumor-mill/scraper")
	ctx, span := tracer.Start(req.Context(), "Scraper.debugHandlerTrackedArticle")
	defer span.End()

	type TmplData struct {
		TextProto string
	}
	tmplData := TmplData{}

	id, err := strconv.ParseUint(path.Base(req.URL.Path), 10, 64)
	if err != nil {
		glog.Errorf("Error while parsing ID: %w", err)
		http.Error(w, "error while parsing ID", http.StatusBadRequest)
		return
	}

	ta, ok, err := s.trackedArticles.Get(ctx, id)
	if err != nil {
		glog.Errorf("Error while getting article: %w", err)
		http.Error(w, "error retrieving article", http.StatusInternalServerError)
		return
	}
	if !ok {
		glog.Errorf("Error: article %d doesn't exist", id)
		http.Error(w, "article doesn't exist", http.StatusNotFound)
		return
	}

	tmplData.TextProto = prototext.Format(ta)

	if err := trackedArticleTemplate.Execute(w, tmplData); err != nil {
		glog.Errorf("Error while executing template: %w", err)
		return
	}
}
