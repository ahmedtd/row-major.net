package site

import (
	"log"
	"net/http"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

type Site struct {
	Mux *http.ServeMux
}

func New(staticContentDir string) *Site {
	s := &Site{
		Mux: http.NewServeMux(),
	}

	log.Printf("serving from %q", staticContentDir)

	h := NewRequestMetricsHandler(http.FileServer(http.Dir(staticContentDir)))
	h.RegisterMetrics()

	s.Mux.Handle("/", h)

	return s
}

type requestMetricsHandler struct {
	inner http.Handler

	requestCount     *stats.Int64Measure
	requestCountView *view.View
}

func NewRequestMetricsHandler(inner http.Handler) *requestMetricsHandler {
	r := &requestMetricsHandler{
		inner: inner,
	}

	r.requestCount = stats.Int64("requests", "", stats.UnitDimensionless)
	r.requestCountView = &view.View{
		Name:        "requests",
		Description: "Counter of requests that have been handled",

		TagKeys: []tag.Key{tag.MustNewKey("path")},

		Measure:     r.requestCount,
		Aggregation: view.Count(),
	}

	return r
}

func (h *requestMetricsHandler) RegisterMetrics() {
	view.Register(h.requestCountView)
}

func (h *requestMetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.inner.ServeHTTP(w, r)

	stats.RecordWithOptions(
		r.Context(),
		stats.WithTags(tag.Insert(tag.MustNewKey("path"), r.URL.Path)),
		stats.WithMeasurements(h.requestCount.M(1)))
}
