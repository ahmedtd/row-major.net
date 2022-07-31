package uitemplates

import "html/template"

type LogInParams struct {
	UserError string
}

var logInText = `{{define "title"}}Log In{{end}}
{{define "breadcrumbs" -}}
<ul class="breadcrumbs"><li class="breadcrumbs-item"><a href="/">Home</a></li><li class="breadcrumbs-item"><a href="/log-in">Log In</a></li></ul>
{{- end}}

{{define "content"}}
Error: {{.UserError}}
<form method="POST">
  <label for="email">Email</label>
  <input type="email" name="email" id="email" required>
  <label for="password">Password</label>
  <input type="password" name="password" id="password" required>
  <input type="submit" value="Log In">
</form>
{{end}}
`

var LogInTemplate = template.Must(template.Must(template.New("base").Parse(baseText)).Parse(logInText))
