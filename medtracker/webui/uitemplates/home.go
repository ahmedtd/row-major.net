package uitemplates

import "html/template"

type HomeParams struct {
	LoggedIn bool
}

var homeText = `{{define "title"}}Home{{end}}
{{define "breadcrumbs" -}}
<ul class="breadcrumbs">
  <li class="breadcrumbs-item">
    <a href="/">Home</a>
  </li>
</ul>
{{- end}}

{{define "content"}}
{{if .LoggedIn}}
<a href="/list-patients">List Patients</a>
{{else}}
<a href="/log-in">Log In</a>
{{end}}
{{end}}
`

var HomeTemplate = template.Must(template.Must(template.New("base").Parse(baseText)).Parse(homeText))
