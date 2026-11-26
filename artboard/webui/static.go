package webui

import (
	"embed"
	"net/http"
)

// staticFiles are served under /static/.  Pages refer to them with the build
// revision in the query string, so they can be cached for a while.
//
//go:embed static
var staticFiles embed.FS

func staticHandler() http.Handler {
	fs := http.FileServerFS(staticFiles)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		fs.ServeHTTP(w, r)
	})
}
