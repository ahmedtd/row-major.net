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
  <p>
    Welcome to MedTracker, a tool that helps you stay on top of renewal dates for your
	or your family members' prescriptions.  Use it when:
	<ul>
	  <li>Your pharmacy doesn't support auto-renew.</li>
	  <li>You need to remind your doctor's office each time a renewal is needed.</li>
	</ul>
  </p>
  
  <p>
    After creating an account, you can add one or more people you are keeping track of,
	and then add information about their prescriptions.  MedTracker will send you reminder
	emails five days and two days before the end of the prescription.  Once you have gotten
	a refill, you can record it in MedTracker to reset the countdown.
  </p>
  
  {{if .LoggedIn}}
    <p>
	  Welcome. Actions available:
	  <ul>
	    <li><a href="/list-patients">List People</a></li>
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
