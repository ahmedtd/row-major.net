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

// type pageBase struct {
// 	baseTemplate *template.Template
// 	logoDataURI  template.URL
// }

// func newPageBase(baseTemplatePath, logoPath string) (*pageBase, error) {
// 	tpl, err := template.ParseFiles(baseTemplatePath)
// 	if err != nil {
// 		return nil, fmt.Errorf("while parsing base template: %w", err)
// 	}

// 	logobytes, err := ioutil.ReadFile(logoPath)
// 	if err != nil {
// 	}

// 	logoDataURI := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(logoBytes)

// 	return &pageBase{
// 		baseTemplate: tpl,
// 		logoDataURI:  logoDataURI,
// 	}
// }

// type StaticPage struct {
// }
