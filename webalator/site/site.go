package site

import (
	"fmt"
	"net/http"

	"row-major/webalator/contentpack"
	"row-major/wordgrid"
)

type Site struct {
	Mux         *http.ServeMux
	ContentPack *contentpack.Handler
}

func New(contentPack *contentpack.Handler) (*Site, error) {
	s := &Site{
		Mux: http.NewServeMux(),
	}

	s.Mux.Handle("/", contentPack)

	// Individual API calls are wired up below.

	wordgridHandler, err := wordgrid.NewHandlerFromFile("wordgrid/sgb-words.txt")
	if err != nil {
		return nil, fmt.Errorf("while creating wordgrid handler: %w", err)
	}
	s.Mux.Handle("/articles/2020-05-12-interactive-word-squares/evaluate", wordgridHandler)

	return s, nil
}
