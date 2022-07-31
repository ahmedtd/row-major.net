package uitemplates

import "html/template"

type HomeParams struct {
	ActiveUser ActiveUserParams
}

var homeText = `{{define "title"}}Home{{end}}
{{define "breadcrumbs" -}}
<ul class="breadcrumbs"><li class="breadcrumbs-item"><a href="/">Home</a></li></ul>
{{- end}}

{{define "content"}}
{{if .ActiveUser.LoggedIn}}
You are now logged in, {{.ActiveUser.Email}}.
{{else}}
<a href="/log-in">Log In</a>
{{end}}
{{end}}
`

var HomeTemplate = template.Must(template.Must(template.New("base").Parse(baseText)).Parse(homeText))
