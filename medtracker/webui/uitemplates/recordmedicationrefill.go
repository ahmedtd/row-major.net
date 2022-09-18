package uitemplates

import "html/template"

type RecordMedicationRefillParams struct {
	PatientID          string
	MedicationName     string
	PatientDisplayName string
	SelfLink           string

	UserError string
}

var recordMedicationRefillText = `
{{define "title"}}Record Medication Refill{{end}}

{{define "breadcrumbs" -}}
<ul class="breadcrumbs"><li class="breadcrumbs-item"><a href="/">Home</a><a href="{{.SelfLink}}">Record Medication Refill</a></li>
{{- end}}

{{define "content"}}

{{if .UserError}}
Error: {{.UserError}}
{{end}}

<form method="POST">
  <label for="patient-id">Patient ID</label>
  <input id="patient-id" type="text" name="patient-id" value={{.PatientID}} required>
  
  <label for="medication-name">Medication Name</label>
  <input id="medication-name" type="text" name="medication-name" value={{.MedicationName}} required>

  <label for="refill-date">Refill Date</label>
  <input id="refill-date" type="text" name="refill-date" required>

  <input type="submit" value="Record">
</form>
{{end}}
`

var RecordMedicationRefillTemplate = template.Must(template.Must(template.New("base").Parse(baseText)).Parse(recordMedicationRefillText))
