// Package sitecontent holds the static files and templates that make up
// row-major.net, embedded into the binary.
package sitecontent

import (
	"embed"
	"io/fs"
)

//go:embed static
var staticFS embed.FS

//go:embed templates
var templatesFS embed.FS

// Static returns the static files, rooted so that the file served at /foo/bar
// is at foo/bar.
func Static() fs.FS {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	return sub
}

// Templates returns the page templates, rooted so that the template served at
// /foo/ is at foo/index.html.tmpl.  The base template is base.html.tmpl.
func Templates() fs.FS {
	sub, err := fs.Sub(templatesFS, "templates")
	if err != nil {
		panic(err)
	}
	return sub
}
