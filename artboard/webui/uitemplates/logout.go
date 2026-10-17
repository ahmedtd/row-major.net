package uitemplates

import (
	"bytes"
	"fmt"
	"html/template"
)

type LogOutParams struct {
}

var logOutText = `
{{define "title"}}Log Out{{end}}

{{define "breadcrumbs" -}}
<li class="breadcrumb-item"><a href="/">Home</a></li>
<li class="breadcrumb-item active" aria-current="page"><a href="/log-out">Log Out</a></li>
{{- end}}

{{define "content"}}
<h1>Log Out</h1>

<form method="POST">
  <button type="submit" class="btn btn-primary">Log Out</button>
</form>
{{end}}
`

var logOutTemplate = template.Must(template.Must(template.New("base").Parse(baseText)).Parse(logOutText))

func LogOutPage(params *LogOutParams) ([]byte, error) {
	b := bytes.Buffer{}
	if err := logOutTemplate.Execute(&b, params); err != nil {
		return nil, fmt.Errorf("while executing template: %w", err)
	}
	return b.Bytes(), nil
}
