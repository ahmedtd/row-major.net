package site

import (
	"fmt"
	"net/http"

	"row-major/wordgrid"
)

type Site struct {
	Mux *http.ServeMux
}

func New(content http.Handler) (*Site, error) {
	s := &Site{
		Mux: http.NewServeMux(),
	}

	s.Mux.Handle("/", content)

	// Individual API calls are wired up below.

	wordgridHandler, err := wordgrid.NewHandlerFromSGBWords()
	if err != nil {
		return nil, fmt.Errorf("while creating wordgrid handler: %w", err)
	}
	s.Mux.Handle("/articles/2020-05-12-interactive-word-squares/evaluate", wordgridHandler)

	return s, nil
}
