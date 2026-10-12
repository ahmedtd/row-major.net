package sitecontent

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path"
	"runtime/debug"
	"strings"
)

// contentTypes gives the content type for each kind of static file we serve.
// We don't rely on mime.TypeByExtension alone, because its answers depend on
// the host's /etc/mime.types, which isn't present in our container images.
var contentTypes = map[string]string{
	".cc":    "text/x-c++src; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".eot":   "application/vnd.ms-fontobject",
	".frag":  "text/plain; charset=utf-8",
	".html":  "text/html; charset=utf-8",
	".jpeg":  "image/jpeg",
	".js":    "text/javascript; charset=utf-8",
	".lua":   "text/x-lua; charset=utf-8",
	".md":    "text/markdown; charset=utf-8",
	".pcap":  "application/vnd.tcpdump.pcap",
	".png":   "image/png",
	".svg":   "image/svg+xml",
	".svgz":  "image/svg+xml-compressed",
	".ttf":   "font/ttf",
	".vert":  "text/plain; charset=utf-8",
	".woff":  "font/woff",
	".woff2": "font/woff2",
}

func contentTypeFor(name string) string {
	ext := path.Ext(name)
	if ct, ok := contentTypes[ext]; ok {
		return ct
	}
	return mime.TypeByExtension(ext)
}

const (
	baseTemplateName  = "base.html.tmpl"
	indexTemplateName = "index.html.tmpl"
)

// Handler serves the site's static files and templates.
//
// A static file at foo/bar is served at /foo/bar.  A template at
// foo/index.html.tmpl is executed on top of base.html.tmpl and served at
// /foo/, with a redirect from /foo.  Paths are matched exactly; there are no
// directory listings.
type Handler struct {
	routes map[string]http.Handler
}

// NewEmbeddedHandler creates a Handler that serves the embedded site content.
func NewEmbeddedHandler() (*Handler, error) {
	return NewHandler(Static(), Templates())
}

// NewHandler creates a Handler that serves static files from staticFS and
// templates from templatesFS.
func NewHandler(staticFS, templatesFS fs.FS) (*Handler, error) {
	return newHandler(staticFS, templatesFS, buildRevision())
}

func newHandler(staticFS, templatesFS fs.FS, revision string) (*Handler, error) {
	h := &Handler{
		routes: map[string]http.Handler{},
	}

	if err := h.addStatics(staticFS); err != nil {
		return nil, fmt.Errorf("while adding static files: %w", err)
	}

	funcs := template.FuncMap{
		"buildRevision": func() string { return revision },
	}

	if err := h.addTemplates(templatesFS, funcs); err != nil {
		return nil, fmt.Errorf("while adding templates: %w", err)
	}

	return h, nil
}

func (h *Handler) addStatics(staticFS fs.FS) error {
	return fs.WalkDir(staticFS, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		sv := &staticFile{
			fsys:        staticFS,
			name:        name,
			contentType: contentTypeFor(name),
		}
		return h.register("/"+name, sv)
	})
}

// buildRevision returns the VCS revision that the binary was built from, as
// stamped by the Go toolchain, with a "-dirty" suffix if the working tree had
// local modifications.
func buildRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	revision := "unknown"
	modified := false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}

	if modified {
		revision += "-dirty"
	}
	return revision
}

func (h *Handler) addTemplates(templatesFS fs.FS, funcs template.FuncMap) error {
	baseText, err := fs.ReadFile(templatesFS, baseTemplateName)
	if err != nil {
		return fmt.Errorf("while reading base template: %w", err)
	}

	return fs.WalkDir(templatesFS, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || name == baseTemplateName {
			return nil
		}
		if path.Base(name) != indexTemplateName {
			return fmt.Errorf("template %v is not named %v", name, indexTemplateName)
		}

		tmpl, err := template.New(name).Funcs(funcs).Parse(string(baseText))
		if err != nil {
			return fmt.Errorf("while parsing base template for %v: %w", name, err)
		}

		specializationText, err := fs.ReadFile(templatesFS, name)
		if err != nil {
			return fmt.Errorf("while reading template %v: %w", name, err)
		}

		if _, err := tmpl.Parse(string(specializationText)); err != nil {
			return fmt.Errorf("while parsing template %v: %w", name, err)
		}

		servingPath := "/"
		if dir := path.Dir(name); dir != "." {
			servingPath = "/" + dir + "/"
		}

		if err := h.register(servingPath, &templatePage{tmpl: tmpl}); err != nil {
			return err
		}

		// Redirect /foo to /foo/.
		if servingPath != "/" {
			redir := http.RedirectHandler(servingPath, http.StatusFound)
			if err := h.register(strings.TrimSuffix(servingPath, "/"), redir); err != nil {
				return err
			}
		}

		return nil
	})
}

func (h *Handler) register(servingPath string, handler http.Handler) error {
	if _, found := h.routes[servingPath]; found {
		return fmt.Errorf("collision at %v", servingPath)
	}
	h.routes[servingPath] = handler
	return nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler, ok := h.routes[r.URL.Path]
	if !ok {
		log.Printf("Didn't find servable for %v", r.URL.Path)
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	handler.ServeHTTP(w, r)
}

type staticFile struct {
	fsys        fs.FS
	name        string
	contentType string
}

func (s *staticFile) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f, err := s.fsys.Open(s.name)
	if err != nil {
		log.Printf("Error while opening static file %v: %v", s.name, err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		log.Printf("Error while statting static file %v: %v", s.name, err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	rs, ok := f.(io.ReadSeeker)
	if !ok {
		log.Printf("Static file %v is not seekable", s.name)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if s.contentType != "" {
		w.Header().Set("Content-Type", s.contentType)
	}
	http.ServeContent(w, r, s.name, info.ModTime(), rs)
}

type templatePage struct {
	tmpl *template.Template
}

func (p *templatePage) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Render to a buffer first, so that a template error results in a clean
	// error response rather than a truncated page.
	buf := &bytes.Buffer{}
	if err := p.tmpl.Execute(buf, nil); err != nil {
		log.Printf("Error while executing template %v: %v", p.tmpl.Name(), err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := buf.WriteTo(w); err != nil {
		log.Printf("Error while writing response for template %v: %v", p.tmpl.Name(), err)
	}
}
