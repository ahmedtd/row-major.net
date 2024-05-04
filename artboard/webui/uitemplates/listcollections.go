package uitemplates

import (
	"bytes"
	"fmt"
	"html/template"
)

type ListCollectionsParams struct {
	Collections []ListCollectionsCollection
}

type ListCollectionsCollection struct {
	DisplayName        string
	ShowCollectionLink string
}

var listCollectionsText = `
{{define "title"}}Collections{{end}}
{{define "breadcrumbs" -}}
  <li class="breadcrumb-item"><a href="/">Home</a></li>
  <li class="breadcrumb-item active" aria-current="page"><a href="/list-collections">List Collections</a></li>
{{- end}}

{{define "content"}}

<table class="table">
  <thead>
    <tr>
      <th scope="col">Name</th>
    </tr>
  </thead>
  <tbody class="table-group-divider">
    {{range .People}}
    <tr>
      <td><a href="{{.ShowCollectionLink}}">{{.DisplayName}}</a></td>
      <td><a href="">Delete</a></td>
    </tr>
{{end}}
</tbody>
</table>

{{end}}
`

var listCollectionsTemplate = template.Must(template.Must(template.New("base").Parse(baseText)).Parse(listCollectionsText))

func ListCollectionsPage(params *ListCollectionsParams) ([]byte, error) {
	b := bytes.Buffer{}
	if err := listCollectionsTemplate.Execute(&b, params); err != nil {
		return nil, fmt.Errorf("while executing template: %w", err)
	}
	return b.Bytes(), nil
}
