package site

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"row-major/wordgrid"
	"strings"
	"sync"
)

type Site struct {
	Mux *http.ServeMux
}

func New(staticContentDir string, templateDir string, rescan bool) (*Site, error) {
	s := &Site{
		Mux: http.NewServeMux(),
	}

	log.Printf("serving from %q", staticContentDir)

	tp, err := newTemplateHandler(templateDir, http.FileServer(http.Dir(staticContentDir)), rescan)
	if err != nil {
		return nil, fmt.Errorf("while creating template handler: %w", err)
	}
	s.Mux.Handle("/", tp)

	wordgridHandler, err := wordgrid.NewHandlerFromFile("wordgrid/sgb-words.txt")
	if err != nil {
		return nil, fmt.Errorf("while creating wordgrid handler: %w", err)
	}
	s.Mux.Handle("/articles/2020-05-12-interactive-word-squares/evaluate", wordgridHandler)

	return s, nil
}

type templateHandler struct {
	lock sync.Mutex
	tpls map[string]*template.Template

	rescan      bool
	templateDir string
	inner       http.Handler
}

func newTemplateHandler(templateDir string, inner http.Handler, rescan bool) (*templateHandler, error) {
	th := &templateHandler{
		tpls:        map[string]*template.Template{},
		rescan:      rescan,
		templateDir: templateDir,
		inner:       inner,
	}
	if err := th.doRescan(); err != nil {
		return nil, err
	}

	return th, nil
}

func (h *templateHandler) doRescan() error {
	baseTemplate := filepath.Join(h.templateDir, "base.html.tmpl")

	return filepath.Walk(h.templateDir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() || info.Name() == "base.html.tmpl" {
			return nil
		}

		tpl, err := template.ParseFiles(baseTemplate, path)
		if err != nil {
			return fmt.Errorf("while parsing template %q: %w", path, err)
		}

		rp := strings.TrimPrefix(filepath.Dir(path)+"/", h.templateDir)
		log.Printf("Registering path %q", rp)

		h.lock.Lock()
		h.tpls[rp] = tpl
		h.lock.Unlock()

		return nil
	})
}

func (h *templateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.rescan {
		h.doRescan()
	}

	h.lock.Lock()
	tpl, ok := h.tpls[r.URL.Path]
	h.lock.Unlock()
	if !ok {
		log.Printf("Didn't find template %q, delegating to inner", r.URL.Path)
		h.inner.ServeHTTP(w, r)
		return
	}

	if err := tpl.Execute(w, nil); err != nil {
		log.Printf("Error while writing http response: %v", err)
	}
}
