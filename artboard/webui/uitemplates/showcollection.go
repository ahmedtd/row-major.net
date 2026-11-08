package uitemplates

import (
	"bytes"
	"fmt"
	"html/template"
)

type ShowCollectionParams struct {
	DisplayName string
	StorageURL  string
	SelfLink    string

	Objects []ShowCollectionObject

	// NextPageLink is empty when there are no more objects.
	NextPageLink string

	// UserError is set if the bucket could not be listed.
	UserError string
}

type ShowCollectionObject struct {
	Name        string
	ContentType string
	Size        string
	Updated     string
}

var showCollectionText = `
{{define "title"}}{{.DisplayName}}{{end}}

{{define "breadcrumbs" -}}
  <li class="breadcrumb-item"><a href="/">Home</a></li>
  <li class="breadcrumb-item active" aria-current="page"><a href="{{.SelfLink}}">{{.DisplayName}}</a></li>
{{- end}}

{{define "content"}}

<h1>{{.DisplayName}}</h1>
<p><code>{{.StorageURL}}</code></p>

{{if .UserError}}
  <div class="alert alert-danger" role="alert">
    Error: {{.UserError}}
  </div>
{{else}}
  {{if .Objects}}
    <table class="table">
      <thead>
        <tr>
          <th scope="col">Name</th>
          <th scope="col">Type</th>
          <th scope="col">Size</th>
          <th scope="col">Updated</th>
        </tr>
      </thead>
      <tbody class="table-group-divider">
      {{range .Objects}}
        <tr>
          <td>{{.Name}}</td>
          <td>{{.ContentType}}</td>
          <td>{{.Size}}</td>
          <td>{{.Updated}}</td>
        </tr>
      {{end}}
      </tbody>
    </table>
  {{else}}
    <p>No objects found.</p>
  {{end}}

  {{if .NextPageLink}}
    <p><a href="{{.NextPageLink}}">Next page</a></p>
  {{end}}
{{end}}

{{end}}
`

var showCollectionTemplate = template.Must(template.Must(template.New("base").Parse(baseText)).Parse(showCollectionText))

func ShowCollectionPage(params *ShowCollectionParams) ([]byte, error) {
	b := bytes.Buffer{}
	if err := showCollectionTemplate.Execute(&b, params); err != nil {
		return nil, fmt.Errorf("while executing template: %w", err)
	}
	return b.Bytes(), nil
}
