package site

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

type Site struct {
	Mux *http.ServeMux
}

func New(staticContentDir string, templateDir string) (*Site, error) {
	s := &Site{
		Mux: http.NewServeMux(),
	}

	log.Printf("serving from %q", staticContentDir)

	rw := newRequestMetricsWrapper()
	rw.RegisterMetrics()

	tp, err := newTemplateHandler(templateDir, http.FileServer(http.Dir(staticContentDir)))
	if err != nil {
		return nil, fmt.Errorf("while creating template handler: %w", err)
	}
	s.Mux.Handle("/", rw.Wrap(tp))

	return s, nil
}

type requestMetricsWrapper struct {
	requestCount     *stats.Int64Measure
	requestCountView *view.View
}

func newRequestMetricsWrapper() *requestMetricsWrapper {
	r := &requestMetricsWrapper{}

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

func (h *requestMetricsWrapper) RegisterMetrics() {
	view.Register(h.requestCountView)
}

func (h *requestMetricsWrapper) Wrap(inner http.Handler) http.Handler {
	return &requestMetricsHandler{
		wrapper: h,
		inner:   inner,
	}
}

type requestMetricsHandler struct {
	wrapper *requestMetricsWrapper
	inner   http.Handler
}

func (h *requestMetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	h.inner.ServeHTTP(w, r)

	log.Printf("Served path=%q", r.URL.Path)

	stats.RecordWithOptions(
		r.Context(),
		stats.WithTags(tag.Insert(tag.MustNewKey("path"), r.URL.Path)),
		stats.WithMeasurements(h.wrapper.requestCount.M(1)))
}

type templateHandler struct {
	tpls  map[string]*template.Template
	inner http.Handler
}

func newTemplateHandler(templateDir string, inner http.Handler) (*templateHandler, error) {
	baseTemplate := filepath.Join(templateDir, "base.html.tmpl")

	th := &templateHandler{
		tpls:  map[string]*template.Template{},
		inner: inner,
	}

	err := filepath.Walk(templateDir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() || info.Name() == "base.html.tmpl" {
			return nil
		}

		tpl, err := template.ParseFiles(baseTemplate, path)
		if err != nil {
			return fmt.Errorf("while parsing template %q: %w", path, err)
		}

		rp := strings.TrimPrefix(filepath.Dir(path)+"/", templateDir)
		log.Printf("Registering path %q", rp)
		th.tpls[rp] = tpl
		return nil
	})
	if err != nil {
		return nil, err
	}

	return th, nil
}

func (h *templateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tpl, ok := h.tpls[r.URL.Path]
	if !ok {
		log.Printf("Didn't find template %q, delegating to inner", r.URL.Path)
		h.inner.ServeHTTP(w, r)
		return
	}

	if err := tpl.Execute(w, nil); err != nil {
		log.Printf("Error while writing http response: %v", err)
	}
}
