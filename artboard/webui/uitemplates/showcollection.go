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

	// CollectionID and SearchLink are used by the tag search form.
	CollectionID string
	SearchLink   string

	// SearchQuery is the active tag search, if any.  When set, Objects only
	// contains objects with a matching tag.
	SearchQuery string

	Objects []ShowCollectionObject

	// NextPageLink is empty when there are no more objects.
	NextPageLink string

	// UserError is set if the bucket could not be listed.
	UserError string
}

type ShowCollectionObject struct {
	Name        string
	ViewLink    string
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

<form method="GET" action="{{.SearchLink}}" class="row g-2 align-items-center mb-3" role="search">
  <input type="hidden" name="id" value="{{.CollectionID}}">
  <div class="col-auto">
    <label for="q" class="visually-hidden">Search tags</label>
    <input id="q" type="search" name="q" value="{{.SearchQuery}}" class="form-control" placeholder="Search tags">
  </div>
  <div class="col-auto">
    <button type="submit" class="btn btn-primary">Search</button>
  </div>
  {{if .SearchQuery}}
  <div class="col-auto">
    <a href="{{.SelfLink}}" class="btn btn-outline-secondary">Clear</a>
  </div>
  {{end}}
</form>

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
          <td><a href="{{.ViewLink}}">{{.Name}}</a></td>
          <td>{{.ContentType}}</td>
          <td>{{.Size}}</td>
          <td>{{.Updated}}</td>
        </tr>
      {{end}}
      </tbody>
    </table>
  {{else}}
    {{if .SearchQuery}}
      <p>No objects have a tag containing &ldquo;{{.SearchQuery}}&rdquo;.</p>
    {{else}}
      <p>No objects found.</p>
    {{end}}
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
