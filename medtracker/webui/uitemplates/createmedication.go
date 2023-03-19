package uitemplates

import "html/template"

type CreateMedicationParams struct {
	PatientID          string
	PatientDisplayName string
	SelfLink           string
	UserError          string
}

var createMedicationText = `
{{define "title"}}Create New Medication{{end}}

{{define "breadcrumbs" -}}
<ul class="breadcrumbs">
  <li class="breadcrumbs-item">
    <a href="/">Home</a>
  </li>
  <li class="breadcrumbs-item">
    <a href="{{.SelfLink}}">Create New Medication</a>
  </li>
</ul>
{{- end}}

{{define "content"}}

{{if .UserError}}
Error: {{.UserError}}
{{end}}

Add new medication for {{.PatientDisplayName}}:
<form method="POST">
  <label for="medication-name">Medication Name</label>
  <input id="medication-name"
         type="text"
		 name="medication-name"
		 value=""
		 required>

  <label for="rx-length-days">Prescription Length (Days)</label>
  <input id="rx-length-days"
         type="number"
         name="rx-length-days"
         value=""
         required>

  <label for="rx-filled-at">Prescription Last Filled On</label>
  <input id="rx-filled-at"
         type="string"
		 name="rx-filled-at"
		 value=""
		 required>

  <input type="submit" value="Add Medication">
</form>

{{end}}
`

var CreateMedicationTemplate = template.Must(template.Must(template.New("base").Parse(baseText)).Parse(createMedicationText))
