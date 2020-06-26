package httpmetrics

import (
	"log"
	"net/http"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

type Wrapper struct {
	requestCount     *stats.Int64Measure
	requestCountView *view.View

	inner http.Handler
}

func New(inner http.Handler) *Wrapper {
	r := &Wrapper{}

	r.requestCount = stats.Int64("requests", "", stats.UnitDimensionless)
	r.requestCountView = &view.View{
		Name:        "requests",
		Description: "Counter of requests that have been handled",

		TagKeys: []tag.Key{},

		Measure:     r.requestCount,
		Aggregation: view.Count(),
	}

	r.inner = inner

	return r
}

func (h *Wrapper) RegisterMetrics() {
	view.Register(h.requestCountView)
}

func (h *Wrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	h.inner.ServeHTTP(w, r)
	finish := time.Now()

	duration := finish.Sub(start)

	log.Printf("Served path=%q useragent=%q remoteaddr=%q duration=%v", r.URL.Path, r.Header["User-Agent"], r.Header["X-Forwarded-For"], duration)

	stats.RecordWithOptions(
		r.Context(),
		stats.WithMeasurements(h.requestCount.M(1)))
}
