package uitemplates

import (
	"bytes"
	"fmt"
	"html/template"
)

type ListPeopleParams struct {
	People []ListPeoplePerson
}

type ListPeoplePerson struct {
	DisplayName      string
	ShowPersonLink   string
	DeletePersonLink string
}

var listPeopleText = `
{{define "title"}}People{{end}}
{{define "breadcrumbs" -}}
  <li class="breadcrumb-item"><a href="/">Home</a></li>
  <li class="breadcrumb-item active" aria-current="page"><a href="/list-patients">List People</a></li>
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
      <td><a href="{{.ShowPersonLink}}">{{.DisplayName}}</a></td>
      <td><a href="{{.DeletePersonLink}}">Delete</a></td>
    </tr>
  {{end}}
  </tbody>
</table>

<a href="/create-person">Add New Person</a>
{{end}}
`

var listPeopleTemplate = template.Must(template.Must(template.New("base").Parse(baseText)).Parse(listPeopleText))

func ListPeoplePage(params *ListPeopleParams) ([]byte, error) {
	b := bytes.Buffer{}
	if err := listPeopleTemplate.Execute(&b, params); err != nil {
		return nil, fmt.Errorf("while executing template: %w", err)
	}
	return b.Bytes(), nil
}
