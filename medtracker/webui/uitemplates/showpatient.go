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
{{define "title"}}Show Patient: {{.DisplayName}}{{end}}

{{define "breadcrumbs" -}}
<ul class="breadcrumbs">
  <li class="breadcrumbs-item">
    <a href="/">Home</a>
  </li>
  <li>
    <a href="/list-patients">List Patients</a>
  </li>
  <li>
    <a href="{{.SelfLink}}">Show Patient: {{.DisplayName}}</a>
  </li>
</ul>
{{- end}}

{{define "content"}}
<h1>Medications</h1>
<table>
  <thead>
    <tr>
	  <th>Medication</th>
	  <th>Days Left</th>
	  <th>Prescription Length (days)</th>
	  <th>Last Filled On</th>
	  <th>Record Refill</th>
	</tr>
  </thead>
  <tbody>
    {{range .Medications}}
    <tr>
	  <td>{{.DisplayName}}</td>
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
