package uitemplates

import (
	"bytes"
	"fmt"
	"html/template"
)

type CreatePersonParams struct {
	UserError string
}

var createPersonText = `
{{define "title"}}Add New Person{{end}}

{{define "breadcrumbs" -}}
  <li class="breadcrumb-item"><a href="/">Home</a></li>
  <li class="breadcrumb-item"><a href="/list-patients">List People</a></li>
  <li class="breadcrumb-item active" aria-current="page"><a href="/create-person">Add New Person</a></li>
{{- end}}

{{define "content"}}

<h1>Add a new person:</h1>

{{if .UserError}}
  <div class="alert alert-danger" role="alert">
    Error: {{.UserError}}
  </div>
{{end}}

<form method="POST">
  <div class="mb-3">
    <label for="name" class="form-label">Name</label>
    <input id="name"
           type="text"
	       name="name"
		   value=""
		   class="form-control"
		   required>
  </div>

  <button type="submit" class="btn btn-primary">Add Person</button>
</form>

{{end}}
`

var createPersonTemplate = template.Must(template.Must(template.New("base").Parse(baseText)).Parse(createPersonText))

func CreatePersonPage(params *CreatePersonParams) ([]byte, error) {
	b := bytes.Buffer{}
	if err := createPersonTemplate.Execute(&b, params); err != nil {
		return nil, fmt.Errorf("while executing template: %w", err)
	}
	return b.Bytes(), nil
}
