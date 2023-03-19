package uitemplates

import "html/template"

type ListPatientsParams struct {
	Patients []ListPatientsPatient
}

type ListPatientsPatient struct {
	DisplayName     string
	ShowPatientLink string
}

var listPatientsText = `{{define "title"}}Patients List{{end}}
{{define "breadcrumbs" -}}
<ul class="breadcrumbs">
  <li class="breadcrumbs-item">
    <a href="/">Home</a>
  </li>
  <li>
    <a href="/list-patients">List Patients</a>
  </li>
</ul>
{{- end}}

{{define "content"}}
<table>
  <thead>
    <tr><td>Patient Name</td></tr>
  </thead>
  <tbody>
  {{range .Patients}}
    <tr><td><a href="{{.ShowPatientLink}}">{{.DisplayName}}</a></td></tr>
  {{end}}
  </tbody>
</table>
{{end}}
`

var ListPatientsTemplate = template.Must(template.Must(template.New("base").Parse(baseText)).Parse(listPatientsText))
