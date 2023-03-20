package uitemplates

import "html/template"

type ShowPatientParams struct {
	DisplayName          string
	SelfLink             string
	CreateMedicationLink string
	Medications          []*ShowPatientMedication
}

type ShowPatientMedication struct {
	DisplayName              string
	RecordRefillLink         string
	PrescriptionDaysLeft     string
	PrescriptionLengthDays   string
	PrescriptionLastFilledOn string
}

var showPatientText = `
{{define "title"}}Show Person: {{.DisplayName}}{{end}}

{{define "breadcrumbs" -}}
  <li class="breadcrumb-item"><a href="/">Home</a></li>
  <li class="breadcrumb-item"><a href="/list-patients">List People</a></li>
  <li class="breadcrumb-item active" aria-current="page"><a href="{{.SelfLink}}">Person: {{.DisplayName}}</a></li>
{{- end}}

{{define "content"}}
<h1>Medications</h1>
<table class="table">
  <thead>
    <tr>
	  <th scope="col">Medication</th>
	  <th scope="col">Days Left</th>
	  <th scope="col">Prescription Length (days)</th>
	  <th scope="col">Last Filled On</th>
	  <th scope="col">Record Refill</th>
	</tr>
  </thead>
  <tbody>
    {{range .Medications}}
    <tr>
	  <th scope="row">{{.DisplayName}}</th>
	  <td>{{.PrescriptionDaysLeft}}</td>
	  <td>{{.PrescriptionLengthDays}}</td>
	  <td>{{.PrescriptionLastFilledOn}}</td>
	  <td><a href="{{.RecordRefillLink}}">Record Refill</a>
	</tr>
	{{end}}
  <tbody>
</table>

<a href="{{.CreateMedicationLink}}">Create New Medication</a>
{{end}}
`

var ShowPatientTemplate = template.Must(template.Must(template.New("base").Parse(baseText)).Parse(showPatientText))
