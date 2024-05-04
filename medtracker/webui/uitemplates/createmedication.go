package uitemplates

import "html/template"

type CreateMedicationParams struct {
	PatientID          string
	PatientDisplayName string
	SelfLink           string
	ShowPatientLink    string
	UserError          string
}

var createMedicationText = `
{{define "title"}}Create New Medication{{end}}

{{define "breadcrumbs" -}}
  <li class="breadcrumb-item"><a href="/">Home</a></li>
  <li class="breadcrumb-item"><a href="/list-patients">List People</a></li>
  <li class="breadcrumb-item"><a href="{{.ShowPatientLink}}">Person: {{.PatientDisplayName}}</a></li>
  <li class="breadcrumb-item active" aria-current="page"><a href="{{.SelfLink}}">Create Medication</a></li>
{{- end}}

{{define "content"}}

<h1>Add New Medication for {{.PatientDisplayName}}:</h1>

{{if .UserError}}
  <div class="alert alert-danger" role="alert">
    Error: {{.UserError}}
  </div>
{{end}}

<form method="POST">
  <div class="mb-3">
    <label for="medication-name" class="form-label">Medication Name</label>
    <input id="medication-name"
           type="text"
	       name="medication-name"
		   value=""
		   class="form-control"
		   required>
  </div>
  
  <div class="mb-3">
    <label for="rx-length-days" class="form-label">Prescription Length (Days)</label>
    <input id="rx-length-days"
           type="number"
           name="rx-length-days"
           value=""
           class="form-control"
           required>
  </div>

  <div class="mb-3">
    <label for="rx-filled-at" class="form-label">Prescription Last Filled On</label>
	<input id="rx-filled-at"
           type="string"
           name="rx-filled-at"
           value=""
           class="form-control"
           required>
  </div>

  <button type="submit" class="btn btn-primary">Add Medication</button>
</form>

{{end}}
`

var CreateMedicationTemplate = template.Must(template.Must(template.New("base").Parse(baseText)).Parse(createMedicationText))
