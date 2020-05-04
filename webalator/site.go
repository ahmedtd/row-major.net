package site

import (
	"encoding/base64"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
)

type Site struct {
	Mux *http.ServeMux
}

func New(templateDir, contentDir string) *Site {
	s := &Site{
		Mux: http.NewServeMux(),
	}

	// pb := newPageBase(path.Join(templateDir, "base.html.in"))

	s.Mux.Register("/", http.FileServer(http.Dir(contentDir)))
}

type pageBase struct {
	baseTemplate *template.Template
	logoDataURI  template.URL
}

func newPageBase(baseTemplatePath, logoPath string) (*pageBase, error) {
	tpl, err := template.ParseFiles(baseTemplatePath)
	if err != nil {
		return nil, fmt.Errorf("while parsing base template: %w", err)
	}

	logobytes, err := ioutil.ReadFile(logoPath)
	if err != nil {
	}

	logoDataURI := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(logoBytes)

	return &pageBase{
		baseTemplate: tpl,
		logoDataURI:  logoDataURI,
	}
}

type StaticPage struct {
}
