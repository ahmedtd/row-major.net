package uitemplates

import "html/template"

type RecordMedicationRefillParams struct {
	PatientID          string
	MedicationName     string
	PatientDisplayName string
	SelfLink           string
	ShowPatientLink    string

	UserError string
}

var recordMedicationRefillText = `
{{define "title"}}Record Medication Refill{{end}}

{{define "breadcrumbs" -}}
  <li class="breadcrumb-item"><a href="/">Home</a></li>
  <li class="breadcrumb-item"><a href="/list-patients">List People</a></li>
  <li class="breadcrumb-item"><a href="{{.ShowPatientLink}}">Person: {{.PatientDisplayName}}</a></li>
  <li class="breadcrumb-item active" aria-current="page"><a href="{{.SelfLink}}">Record Medication Refill</a></li>
{{- end}}

{{define "content"}}

{{if .UserError}}
  <div class="alert alert-danger" role="alert">
    Error: {{.UserError}}
  </div>
{{end}}

<form method="POST">
  <div class="mb-3">
    <label for="refill-date" class="form-label">Refill Date</label>
    <input id="refill-date" type="text" name="refill-date" class="form-control" required>
  </div>

  <button type="submit" class="btn btn-primary">Record Refill</button>
</form>
{{end}}
`

var RecordMedicationRefillTemplate = template.Must(template.Must(template.New("base").Parse(baseText)).Parse(recordMedicationRefillText))
