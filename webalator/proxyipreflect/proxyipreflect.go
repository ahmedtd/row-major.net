package proxyipreflect

import (
	"encoding/json"
	"net/http"
)

type Handler struct {
}

func New() *Handler {
	return &Handler{}
}

type Result struct {
	Headers map[string][]string
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	result := &Result{
		Headers: map[string][]string{},
	}
	result.Headers["X-Forwarded-For"] = r.Header["X-Forwarded-For"]

	data, err := json.Marshal(result)
	if err != nil {
		http.Error(w, "failed to marshal json", http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(data); err != nil {
		return
	}
}
