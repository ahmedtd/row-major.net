package uitemplates

import "html/template"

type LogInParams struct {
	GoogleOAuthClientID  string
	SignInWithGoogleLink string
}

var logInText = `{{define "title"}}Log In{{end}}
{{define "breadcrumbs" -}}
  <li class="breadcrumb-item"><a href="/">Home</a></li>
  <li class="breadcrumb-item active" aria-current="page"><a href="/log-in">Log In</a></li>
 {{- end}}

{{define "content"}}
<div id="g_id_onload"
     data-client_id="{{.GoogleOAuthClientID}}"
     data-login_uri="{{.SignInWithGoogleLink}}"
     data-auto_prompt="false">
</div>
<div class="g_id_signin"
     data-type="standard"
     data-size="large"
     data-theme="outline"
     data-text="sign_in_with"
     data-shape="rectangular"
     data-logo_alignment="left">
</div>
{{end}}

{{define "scripts"}}
<script src="https://accounts.google.com/gsi/client" async defer></script>
{{end}}
`

var LogInTemplate = template.Must(template.Must(template.New("base").Parse(baseText)).Parse(logInText))
