package site

import (
	"log"
	"net/http"
)

type Site struct {
	Mux *http.ServeMux
}

func New(staticContentDir string) *Site {
	s := &Site{
		Mux: http.NewServeMux(),
	}

	log.Printf("serving from %q", staticContentDir)
	s.Mux.Handle("/", http.FileServer(http.Dir(staticContentDir)))

	return s
}
