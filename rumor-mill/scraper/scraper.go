// Package scraper houses the logic for determining which stories are
// of interest.
package scraper

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	texttemplate "text/template"
	"time"

	"row-major/rumor-mill/hackernews"

	"cloud.google.com/go/firestore"
	"github.com/golang/glog"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"google.golang.org/api/iterator"
)

type TrackedArticle struct {
	ID string `firestore:"id"`

	// A Hacker News ID.
	HackerNewsID string `firestore:"hackerNewsID"`

	FirstSeenTime  time.Time `firestore:"firstSeenTime"`
	LatestSeenTime time.Time `firestore:"latestSeenTime"`

	LatestRank int64 `firestore:"latestRank"`

	Title     string `firestore:"title"`
	Submitter string `firestore:"submitter"`

	HaveCheckedWatchConfigs bool     `firestore:"haveCheckedWatchConfigs"`
	CheckedWatchConfigs     []string `firestore:"checkedWatchConfigs"`
	MatchedWatchConfigs     []string `firestore:"matchedWatchConfigs"`

	Expiry time.Time `firestore:"expiry"`
}

type WatchConfig struct {
	ID string `firestore:"id"`

	// A friendly descriptor for the watch config.  Appears in alert emails.
	Description string `firestore:"description"`

	TopicRegexp string `firestore:"topicRegexp"`

	NotifyAddresses []string `firestore:"notifyAddresses"`
}

func hnURL(t *TrackedArticle) string {
	return fmt.Sprintf("https://news.ycombinator.com/item?id=%s", t.HackerNewsID)
}

type hnClient interface {
	TopStories(context.Context) ([]uint64, error)
	Item(context.Context, uint64) (*hackernews.Item, error)
	Items(context.Context, []uint64) ([]*hackernews.Item, error)
}

// Scraper checks data sources for articles matching the specified topic regexp.
type Scraper struct {
	hn              hnClient
	sg              *sendgrid.Client
	firestoreClient *firestore.Client

	scrapePeriod time.Duration
}

type ScraperOpt func(*Scraper)

func WithScrapePeriod(period time.Duration) ScraperOpt {
	return func(s *Scraper) {
		s.scrapePeriod = period
	}
}

// New creates a new Scraper
func New(hn hnClient, sg *sendgrid.Client, firestoreClient *firestore.Client, opts ...ScraperOpt) *Scraper {
	scraper := &Scraper{
		hn:              hn,
		sg:              sg,
		firestoreClient: firestoreClient,
		scrapePeriod:    30 * time.Minute,
	}

	for _, opt := range opts {
		opt(scraper)
	}

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

	err := s.firestoreClient.RunTransaction(ctx, func(ctx context.Context, txn *firestore.Transaction) error {
		now := time.Now()

		var articleSnapshot *firestore.DocumentSnapshot
		articleIter := s.firestoreClient.Collection("TrackedArticles").Where("hackerNewsID", "==", strconv.FormatUint(id, 10)).Documents(ctx)
		for {
			var err error
			articleSnapshot, err = articleIter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return fmt.Errorf("while querying for tracked articles with HN id %d: %w", id, err)
			}

			// This field is supposed to be unique.  Consider only one document.
			break
		}
		if articleSnapshot == nil {
			// Not found in DB.  Create.
			item, err := s.hn.Item(ctx, id)
			if err != nil {
				return fmt.Errorf("while fetching item %d from HN: %w", id, err)
			}

			newArticleRef := s.firestoreClient.Collection("TrackedArticles").NewDoc()

			trackedArticle := &TrackedArticle{
				ID:             newArticleRef.ID,
				HackerNewsID:   strconv.FormatUint(id, 10),
				FirstSeenTime:  now,
				LatestSeenTime: now,
				LatestRank:     int64(rank) + 1,
				Title:          item.Title,
				Submitter:      item.By,
				Expiry:         time.Now().Add(12 * time.Hour),
			}

			if err := txn.Create(newArticleRef, trackedArticle); err != nil {
				return fmt.Errorf("while writing new tracked article: %w", err)
			}

			return nil
		}

		// Found in DB. Update.

		trackedArticle := &TrackedArticle{}
		if err := articleSnapshot.DataTo(trackedArticle); err != nil {
			return fmt.Errorf("while deserializing tracked article: %w", err)
		}

		trackedArticle.LatestSeenTime = now
		trackedArticle.LatestRank = int64(rank) + 1
		trackedArticle.Expiry = time.Now().Add(12 * time.Hour)

		if err := txn.Set(articleSnapshot.Ref, trackedArticle); err != nil {
			return fmt.Errorf("while writing back tracked article: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("while running Firestore transaction: %w", err)
	}

	return nil
}

//
func (s *Scraper) sendAlerts(ctx context.Context) error {
	tracer := otel.Tracer("row-major/rumor-mill/scraper")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "Scraper.sendAlerts")
	defer span.End()

	watchConfigs := []*WatchConfig{}
	watchConfigIter := s.firestoreClient.Collection("WatchConfigs").Documents(ctx)
	for {
		watchConfigSnapshot, err := watchConfigIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("while iterating watchconfigs: %w", err)
		}

		watchConfig := &WatchConfig{}
		if err := watchConfigSnapshot.DataTo(watchConfig); err != nil {
			return fmt.Errorf("while unmarshaling watchconfig: %w", err)
		}

		watchConfigs = append(watchConfigs, watchConfig)
	}

	// Use errgroup and semaphore to limit concurrency.
	eg, ctx := errgroup.WithContext(ctx)
	sem := semaphore.NewWeighted(500)

	trackedArticleIter := s.firestoreClient.Collection("TrackedArticles").Where("haveCheckedWatchConfigs", "==", false).Documents(ctx)
	for {
		trackedArticleSnapshot, err := trackedArticleIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("while iterating tracked articles: %w", err)
		}

		trackedArticle := &TrackedArticle{}
		if err := trackedArticleSnapshot.DataTo(trackedArticle); err != nil {
			return fmt.Errorf("while unmarshaling tracked article: %w", err)
		}

		if err := sem.Acquire(ctx, 1); err != nil {
			return fmt.Errorf("while acquiring concurrency limiter semaphore: %w", err)
		}

		eg.Go(func() error {
			var span trace.Span
			ctx, span = tracer.Start(ctx, "Scraper.sendAlertsForArticle")
			defer span.End()

			defer sem.Release(1)

			for _, wc := range watchConfigs {
				if err := s.sendAlertForArticleAndWatchConfig(ctx, trackedArticle, wc); err != nil {
					return fmt.Errorf("while checking article %s against watchconfig %s: %w", trackedArticleSnapshot.Ref.ID, wc.ID, err)
				}
			}

			trackedArticle.HaveCheckedWatchConfigs = true

			// sendAlertsForArticle might have updated the set of checked or matched watch configs.
			_, err := trackedArticleSnapshot.Ref.Set(ctx, trackedArticle)
			if err != nil {
				return fmt.Errorf("while updating article %s: %w", trackedArticleSnapshot.Ref.ID, err)
			}

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("while waiting for completion of errgroup: %w", err)
	}

	return nil
}

const emailPlain = `There's a new Hacker News article matching your watch config:
* Article: {{.ArticleTitle}}
* Link: {{.ArticleLink}}
* Watch Config: {{.WatchConfigDescription}}
`

var emailPlainTemplate = texttemplate.Must(texttemplate.New("email").Parse(emailPlain))

func (s *Scraper) sendAlertForArticleAndWatchConfig(ctx context.Context, ta *TrackedArticle, wc *WatchConfig) error {
	// Have we already fired an alert for this watch config?
	for _, wcID := range ta.CheckedWatchConfigs {
		if wcID == wc.ID {
			return nil
		}
	}

	if ta.LatestRank >= 500 {
		return nil
	}

	r, err := regexp.Compile(wc.TopicRegexp)
	if err != nil {
		return fmt.Errorf("while compiling WatchConfig regexp: %w", err)
	}

	ta.CheckedWatchConfigs = append(ta.CheckedWatchConfigs, wc.ID)
	if !r.MatchString(strings.ToLower(ta.Title)) {
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
	if err := emailPlainTemplate.Execute(textContent, params); err != nil {
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

	ta.MatchedWatchConfigs = append(ta.MatchedWatchConfigs, wc.ID)

	return nil
}

func (s *Scraper) RegisterDebugHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/rumor-mill/tracked-articles", s.debugHandlerTrackedArticles)
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
		glog.Errorf("Error while retrieving tracked article data: %v", err)
		http.Error(w, "error retrieving tracked article data", http.StatusInternalServerError)
		return
	}

	if err := articlesTemplate.Execute(w, tmplData); err != nil {
		glog.Errorf("Error while executing template: %v", err)
		return
	}
}

func (s *Scraper) debugHandlerTrackedArticlesData(ctx context.Context) (*TrackedArticlesData, error) {
	tmplData := &TrackedArticlesData{}

	trackedArticleIter := s.firestoreClient.Collection("TrackedArticles").Documents(ctx)
	for {
		trackedArticleSnapshot, err := trackedArticleIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("while iterating tracked articles: %w", err)
		}

		ta := &TrackedArticle{}
		if err := trackedArticleSnapshot.DataTo(ta); err != nil {
			return nil, fmt.Errorf("while unmarshaling tracked article: %w", err)
		}

		tmplArticle := TrackedArticleData{
			LatestRank: ta.LatestRank,
			Title:      ta.Title,
			Submitter:  ta.Submitter,
			URL:        hnURL(ta),
		}

		if len(ta.MatchedWatchConfigs) != 0 {
			tmplData.InterestedArticles = append(tmplData.InterestedArticles, tmplArticle)
		} else {
			tmplData.NotInterestedArticles = append(tmplData.NotInterestedArticles, tmplArticle)
		}
	}

	sort.Slice(tmplData.InterestedArticles, func(i, j int) bool {
		return tmplData.InterestedArticles[i].LatestRank < tmplData.InterestedArticles[j].LatestRank
	})
	sort.Slice(tmplData.NotInterestedArticles, func(i, j int) bool {
		return tmplData.NotInterestedArticles[i].LatestRank < tmplData.NotInterestedArticles[j].LatestRank
	})

	return tmplData, nil
}
