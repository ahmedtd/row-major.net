package uitemplates

import "html/template"

type ListPatientsParams struct {
	Patients []ListPatientsPatient
}

type ListPatientsPatient struct {
	DisplayName     string
	ShowPatientLink string
}

var listPatientsText = `
{{define "title"}}People{{end}}
{{define "breadcrumbs" -}}
  <li class="breadcrumb-item"><a href="/">Home</a></li>
  <li class="breadcrumb-item active" aria-current="page"><a href="/list-patients">List People</a></li>
{{- end}}

{{define "content"}}

<table class="table">
  <thead>
    <tr>
      <th scope="col">Name</th>
    </tr>
  </thead>
  <tbody class="table-group-divider">
  {{range .Patients}}
    <tr>
      <td><a href="{{.ShowPatientLink}}">{{.DisplayName}}</a></td>
    </tr>
  {{end}}
  </tbody>
</table>

<a href="/create-person">Add New Person</a>
{{end}}
`

var ListPatientsTemplate = template.Must(template.Must(template.New("base").Parse(baseText)).Parse(listPatientsText))
