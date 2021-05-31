// Package scraper houses the logic for determining which stories are
// of interest.
package scraper

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"row-major/rumor-mill/hackernews"
	trackerpb "row-major/rumor-mill/scraper/trackerpb"

	"github.com/golang/glog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
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
	stateLock         sync.Mutex
	hn                hnClient
	hnTrackedArticles map[uint64]*trackerpb.TrackedArticle

	watchConfigs map[uint64]*WatchConfig

	checkpointWrite   func(context.Context, *Scraper) error
	checkpointRestore func(context.Context, *Scraper) error
}

type ScraperOpt func(*Scraper)

func WithGCSCheckpointFile(path string) ScraperOpt {
	return func(s *Scraper) {
	}
}

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
func New(hn hnClient, opts ...ScraperOpt) *Scraper {
	scraper := &Scraper{
		hn:                hn,
		hnTrackedArticles: map[uint64]*trackerpb.TrackedArticle{},

		watchConfigs: map[uint64]*WatchConfig{},

		checkpointWrite: func(ctx context.Context, s *Scraper) error {
			return nil
		},
		checkpointRestore: func(ctx context.Context, s *Scraper) error {
			return nil
		},
	}

	for _, opt := range opts {
		opt(scraper)
	}

	// TODO: Validate watchConfigs (unique IDs).

	return scraper
}

// Run starts the Scraper's loop.
func (s *Scraper) Run(ctx context.Context) {
	s.stateLock.Lock()
	if err := s.checkpointRestore(ctx, s); err != nil {
		glog.Errorf("Error while restoring from checkpoint: %v", err)
	}
	s.stateLock.Unlock()

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
	ctx, span = tracer.Start(ctx, "Scraper Pass")
	defer span.End()

	s.stateLock.Lock()
	defer s.stateLock.Unlock()

	if err := s.ingestTopStories(ctx); err != nil {
		return fmt.Errorf("while scraping: %w", err)
	}

	alerts, err := s.tickStates(ctx)
	if err != nil {
		return fmt.Errorf("while ticking states of tracked articles: %w", err)
	}

	if err := s.sendAlerts(ctx, alerts); err != nil {
		return fmt.Errorf("while sending alerts: %w", err)
	}

	s.reapArticles()

	s.updateMetrics(ctx)

	if err := s.checkpointWrite(ctx, s); err != nil {
		return fmt.Errorf("while writing checkpoint: %w", err)
	}

	return nil
}

func (s *Scraper) ingestTopStories(ctx context.Context) error {
	tracer := otel.Tracer("row-major/rumor-mill/scraper")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "Ingest Top Stories")
	defer span.End()

	topStories, err := s.hn.TopStories(ctx)
	if err != nil {
		return fmt.Errorf("while querying for top stories: %w", err)
	}

	// Set all articles to have a sentinel rank.  A later step will move all
	// articles with the sentinel rank into TrackerStateToReap.
	for _, ta := range s.hnTrackedArticles {
		ta.ReapAllowed = true
	}

	// Fill in real ranks, transparently creating new articles as necessary
	for rank, id := range topStories {
		ta, ok := s.hnTrackedArticles[id]

		if !ok {
			// We don't know about this article
			ta = &trackerpb.TrackedArticle{
				Id:              id,
				FetchRequired:   true,
				CurTrackerState: trackerpb.TrackerState_UNTRACKED,
				FirstSeenTime:   time.Now().UnixNano(),
			}
			s.hnTrackedArticles[id] = ta
		}

		ta.LatestSeenTime = time.Now().UnixNano()
		ta.Rank = int64(rank) + 1
		ta.ReapAllowed = false
	}

	// We need to fetch articles that that have FetchRequired set
	idsToFetch := []uint64{}
	for id, ta := range s.hnTrackedArticles {
		if ta.FetchRequired {
			idsToFetch = append(idsToFetch, id)
		}
	}

	items, err := s.hn.Items(ctx, idsToFetch)
	if err != nil {
		return fmt.Errorf("while fetching story items: %w", err)
	}

	for _, item := range items {
		ta := s.hnTrackedArticles[item.ID]

		ta.Title = item.Title
		ta.Submitter = item.By
		ta.FetchRequired = false
	}

	return nil
}

type alertType int

const (
	alertTypeNew alertType = iota
)

type alertEvent struct {
	id            uint64
	at            alertType
	watchConfigID uint64
}

func (s *Scraper) tickStates(ctx context.Context) ([]alertEvent, error) {
	alerts := []alertEvent{}
	for id, ta := range s.hnTrackedArticles {
		switch ta.CurTrackerState {
		case trackerpb.TrackerState_UNTRACKED:
			if ta.GetFetchRequired() == false { // We can only make a decision once the article has actually been fetched.
				for _, wc := range s.watchConfigs {
					if wc.TopicRegexp.MatchString(strings.ToLower(ta.Title)) {
						ta.InterestedWatchConfigs = append(ta.InterestedWatchConfigs, wc.ID)
						alerts = append(alerts, alertEvent{id: id, at: alertTypeNew, watchConfigID: wc.ID})
					}
				}
			}

			if len(ta.InterestedWatchConfigs) > 0 {
				ta.CurTrackerState = trackerpb.TrackerState_INTERESTED
			} else {
				ta.CurTrackerState = trackerpb.TrackerState_NOT_INTERESTED
			}
		case trackerpb.TrackerState_INTERESTED:
		case trackerpb.TrackerState_NOT_INTERESTED:
			// Do nothing.
		default:
			panic(fmt.Sprintf("unhandled TrackerState %d", ta.CurTrackerState))
		}
	}

	return alerts, nil
}

func (s *Scraper) updateMetrics(ctx context.Context) {
	// trackedArticles.RemoveAll()
	// trackedArticleRanks.RemoveAll()
	// for id, ta := range s.hnTrackedArticles {
	// 	trackedArticles.Set(1, int(id), ta.GetTitle(), ta.GetCurTrackerState().String())
	// 	trackedArticleRanks.Set(ta.GetRank(), int(id), ta.GetTitle(), ta.GetCurTrackerState().String())
	// }
}

func (s *Scraper) sendAlerts(ctx context.Context, alerts []alertEvent) error {
	for _, ev := range alerts {
		ta := s.hnTrackedArticles[ev.id]

		if ev.at == alertTypeNew {
			glog.Infof("New interesting top story: %d %q https://news.ycombinator.com/item?id=%d", ta.GetRank(), ta.GetTitle(), ta.GetId())

			wc, ok := s.watchConfigs[ev.watchConfigID]
			if !ok {
				return fmt.Errorf("invalid watch config ID %d", ev.watchConfigID)
			}

			// TODO: Notify wc.NotifyAddresses about `ta`.
			glog.Infof("I would have notified %v about %v", wc.NotifyAddresses, ta)
		}
	}

	return nil
}

func (s *Scraper) reapArticles() {
	for id, ta := range s.hnTrackedArticles {
		if ta.ReapAllowed {
			delete(s.hnTrackedArticles, id)
		}
	}
}

const articlesHTML = `
<!DOCTYPE html>
<head>
	<title>Help</title>
</head>

<h1>Untracked Articles</h1>
<ul>
{{range .UntrackedArticles}}
<li>({{.Rank}}) <a href="{{.URL}}">{{.Title}}</a>; submitted by {{.Submitter}}</li>
{{end}}
</ul>

<h1>Interested Articles</h1>
<ul>
{{range .InterestedArticles}}
<li>({{.Rank}}) <a href="{{.URL}}">{{.Title}}</a>; submitted by {{.Submitter}}</li>
{{end}}
</ul>

<h1>Not Interested Articles</h1>
<ul>
{{range .NotInterestedArticles}}
<li>({{.Rank}}) <a href="{{.URL}}">{{.Title}}</a>; submitted by {{.Submitter}}</li>
{{end}}
</ul>

`

var articlesTemplate = template.Must(template.New("articles").Parse(articlesHTML))

func (s *Scraper) RegisterDebugHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/rumor_mill/articles", func(w http.ResponseWriter, req *http.Request) {
		type TmplArticle struct {
			Rank      int64
			Title     string
			Submitter string
			URL       string
			State     string
		}
		type TmplData struct {
			UntrackedArticles     []TmplArticle
			InterestedArticles    []TmplArticle
			NotInterestedArticles []TmplArticle
			ToReapArticles        []TmplArticle
		}
		tmplData := TmplData{}
		for _, ta := range s.hnTrackedArticles {
			article := TmplArticle{
				Rank:      ta.Rank,
				Title:     ta.Title,
				Submitter: ta.Submitter,
				URL:       hnURL(ta),
				State:     ta.CurTrackerState.String(),
			}

			switch ta.CurTrackerState {
			case trackerpb.TrackerState_UNTRACKED:
				tmplData.UntrackedArticles = append(tmplData.UntrackedArticles, article)
			case trackerpb.TrackerState_INTERESTED:
				tmplData.InterestedArticles = append(tmplData.InterestedArticles, article)
			case trackerpb.TrackerState_NOT_INTERESTED:
				tmplData.NotInterestedArticles = append(tmplData.NotInterestedArticles, article)
			}

			sort.Slice(tmplData.UntrackedArticles, func(i, j int) bool {
				return tmplData.UntrackedArticles[i].Rank < tmplData.UntrackedArticles[j].Rank
			})
			sort.Slice(tmplData.InterestedArticles, func(i, j int) bool {
				return tmplData.InterestedArticles[i].Rank < tmplData.InterestedArticles[j].Rank
			})
			sort.Slice(tmplData.NotInterestedArticles, func(i, j int) bool {
				return tmplData.NotInterestedArticles[i].Rank < tmplData.NotInterestedArticles[j].Rank
			})
			sort.Slice(tmplData.ToReapArticles, func(i, j int) bool {
				return tmplData.ToReapArticles[i].Rank < tmplData.ToReapArticles[j].Rank
			})
		}

		articlesTemplate.Execute(w, tmplData)
	})
}
