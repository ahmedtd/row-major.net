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
    <p>
	  Welcome. Actions available:
	  <ul>
	    <li><a href="/list-collections">List Collections</a></li>
		<li><a href="/log-out">Log Out</a></li>
	  </ul>
	</p>
  {{else}}
    <p>
      To get started, <a href="/log-in">Log In</a>.
    </p>
  {{end}}
{{end}}
`

var HomeTemplate = template.Must(template.Must(template.New("base").Parse(baseText)).Parse(homeText))
