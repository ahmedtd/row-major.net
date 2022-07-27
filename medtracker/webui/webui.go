package webui

import (
	"net/http"

	"cloud.google.com/go/firestore"
)

type WebUI struct {
	firestoreClient *firestore.Client
}

func New(firestoreClient *firestore.Client) *WebUI {
	return &WebUI{
		firestoreClient: firestoreClient,
	}
}

func (w *WebUI) Register(m *http.ServeMux) {
	m.HandleFunc("/", w.home)
}

func (w *WebUI) home(w http.ResponseWriter, r *http.Request) error {
}
