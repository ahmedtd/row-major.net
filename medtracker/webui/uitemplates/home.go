package uitemplates

import "html/template"

type HomeParams struct {
	LoggedIn bool
}

var homeText = `{{define "title"}}Home{{end}}
{{define "breadcrumbs" -}}
  <li class="breadcrumb-item active" aria-current="page"><a href="/">Home</a></li>
{{- end}}

{{define "content"}}
{{if .LoggedIn}}
<a href="/list-patients">List People</a>
{{else}}
<a href="/log-in">Log In</a>
{{end}}
{{end}}
`

var HomeTemplate = template.Must(template.Must(template.New("base").Parse(baseText)).Parse(homeText))
