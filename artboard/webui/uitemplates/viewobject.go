package uitemplates

import (
	"bytes"
	"fmt"
	"html/template"
)

type ViewObjectParams struct {
	CollectionDisplayName string
	CollectionLink        string

	// Name is the object's name relative to the collection.
	Name        string
	SelfLink    string
	ContentLink string
	ContentType string
	Size        string
	Updated     string

	IsImage bool
	IsVideo bool

	// PrevLink and NextLink are empty at the ends of the collection.
	PrevLink string
	NextLink string

	Tags []string

	// AddTagLink is the target of the add-tag form.  CollectionID is passed
	// along with Name as hidden form fields.
	AddTagLink   string
	CollectionID string

	// UserError is shown above the add-tag form, and TagInput pre-fills it.
	UserError string
	TagInput  string

	// SetCaptionLink is the target of the caption form, which is pre-filled
	// with CaptionInput.  CaptionError is shown above it.
	SetCaptionLink string
	CaptionError   string
	CaptionInput   string
}

var viewObjectText = `
{{define "title"}}{{.Name}}{{end}}

{{define "breadcrumbs" -}}
  <li class="breadcrumb-item"><a href="/">Home</a></li>
  <li class="breadcrumb-item"><a href="{{.CollectionLink}}">{{.CollectionDisplayName}}</a></li>
  <li class="breadcrumb-item active" aria-current="page"><a href="{{.SelfLink}}">{{.Name}}</a></li>
{{- end}}

{{define "pager"}}
<nav aria-label="Object navigation" class="d-flex justify-content-between my-3">
  {{if .PrevLink}}
    <a class="btn btn-outline-primary" href="{{.PrevLink}}" rel="prev">&larr; Previous</a>
  {{else}}
    <a class="btn btn-outline-primary disabled" aria-disabled="true">&larr; Previous</a>
  {{end}}
  {{if .NextLink}}
    <a class="btn btn-outline-primary" href="{{.NextLink}}" rel="next">Next &rarr;</a>
  {{else}}
    <a class="btn btn-outline-primary disabled" aria-disabled="true">Next &rarr;</a>
  {{end}}
</nav>
{{end}}

{{define "content"}}

<h1 class="text-break">{{.Name}}</h1>

{{template "pager" .}}

<div class="text-center">
  {{if .IsImage}}
    <a href="{{.ContentLink}}"><img src="{{.ContentLink}}" alt="{{.Name}}" class="img-fluid" style="max-height: 80vh"></a>
  {{else if .IsVideo}}
    <video src="{{.ContentLink}}" controls preload="metadata" class="mw-100" style="max-height: 80vh"></video>
  {{else}}
    <p>This file can't be previewed. <a href="{{.ContentLink}}">Open it directly.</a></p>
  {{end}}
</div>

{{template "pager" .}}

<dl class="row">
  <dt class="col-sm-2">Type</dt><dd class="col-sm-10">{{.ContentType}}</dd>
  <dt class="col-sm-2">Size</dt><dd class="col-sm-10">{{.Size}}</dd>
  <dt class="col-sm-2">Updated</dt><dd class="col-sm-10">{{.Updated}}</dd>
</dl>

<h2 id="caption" class="h4">Caption</h2>

{{if .CaptionError}}
  <div class="alert alert-danger" role="alert">
    Error: {{.CaptionError}}
  </div>
{{end}}

<form method="POST" action="{{.SetCaptionLink}}" class="mb-4">
  <input type="hidden" name="collection" value="{{.CollectionID}}">
  <input type="hidden" name="name" value="{{.Name}}">
  <div class="mb-2">
    <label for="caption-input" class="visually-hidden">Caption</label>
    <textarea id="caption-input" name="caption" rows="3" class="form-control" placeholder="No caption">{{.CaptionInput}}</textarea>
  </div>
  <button type="submit" class="btn btn-primary">Save Caption</button>
</form>

<h2 id="tags" class="h4">Tags</h2>

{{if .Tags}}
<table class="table table-sm">
  <thead>
    <tr><th scope="col">Tag</th></tr>
  </thead>
  <tbody>
    {{range .Tags}}
    <tr><td>{{.}}</td></tr>
    {{end}}
  </tbody>
</table>
{{else}}
<p class="text-body-secondary">No tags.</p>
{{end}}

{{if .UserError}}
  <div class="alert alert-danger" role="alert">
    Error: {{.UserError}}
  </div>
{{end}}

<form method="POST" action="{{.AddTagLink}}" class="row g-2 align-items-center">
  <input type="hidden" name="collection" value="{{.CollectionID}}">
  <input type="hidden" name="name" value="{{.Name}}">
  <div class="col-auto">
    <label for="tag" class="visually-hidden">Tag</label>
    <input id="tag" type="text" name="tag" value="{{.TagInput}}" class="form-control" placeholder="New tag" required>
  </div>
  <div class="col-auto">
    <button type="submit" class="btn btn-primary">Add Tag</button>
  </div>
</form>

{{end}}
`

var viewObjectTemplate = template.Must(template.Must(template.New("base").Parse(baseText)).Parse(viewObjectText))

func ViewObjectPage(params *ViewObjectParams) ([]byte, error) {
	b := bytes.Buffer{}
	if err := viewObjectTemplate.Execute(&b, params); err != nil {
		return nil, fmt.Errorf("while executing template: %w", err)
	}
	return b.Bytes(), nil
}
