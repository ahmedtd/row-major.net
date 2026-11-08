package uitemplates

import (
	"bytes"
	"fmt"
	"html/template"
)

type CreateCollectionParams struct {
	UserError string

	// Previously-entered values, so the user doesn't have to retype them after
	// an error.
	DisplayName string
	StorageURL  string
}

var createCollectionText = `
{{define "title"}}Create Collection{{end}}

{{define "breadcrumbs" -}}
  <li class="breadcrumb-item"><a href="/">Home</a></li>
  <li class="breadcrumb-item active" aria-current="page"><a href="/create-collection">Create Collection</a></li>
{{- end}}

{{define "content"}}

<h1>Create a collection</h1>

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
           value="{{.DisplayName}}"
           class="form-control"
           required>
  </div>

  <div class="mb-3">
    <label for="storage-url" class="form-label">Cloud Storage URL</label>
    <input id="storage-url"
           type="text"
           name="storage-url"
           value="{{.StorageURL}}"
           placeholder="gs://my-bucket/optional/folder"
           pattern="gs://.+"
           class="form-control"
           aria-describedby="storage-url-help"
           required>
    <div id="storage-url-help" class="form-text">
      The bucket (and optional folder) holding your media.  Grant this site's
      service identity read access to the bucket; no keys are needed.
    </div>
  </div>

  <button type="submit" class="btn btn-primary">Create Collection</button>
</form>

{{end}}
`

var createCollectionTemplate = template.Must(template.Must(template.New("base").Parse(baseText)).Parse(createCollectionText))

func CreateCollectionPage(params *CreateCollectionParams) ([]byte, error) {
	b := bytes.Buffer{}
	if err := createCollectionTemplate.Execute(&b, params); err != nil {
		return nil, fmt.Errorf("while executing template: %w", err)
	}
	return b.Bytes(), nil
}
