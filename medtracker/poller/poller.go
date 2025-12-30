package poller

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"text/template"
	"time"

	"row-major/medtracker/dbtypes"
	"row-major/medtracker/webui"

	"cloud.google.com/go/firestore"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"google.golang.org/api/iterator"
)

type MedicationAlert struct {
	NotifyUsers              []string
	PrescriptionLengthAlerts []PrescriptionLengthAlert
	ShowPatientLink          string
}

type PrescriptionLengthAlert struct {
	Name                     string
	Info                     string
	PrescriptionLastFilledAt time.Time
	PrescriptionLengthDays   int64
}

// Poller runs an infinite loop,
type Poller struct {
	firestoreClient *firestore.Client
	sendgridClient  *sendgrid.Client
	recheckPeriod   time.Duration
}

func New(firestoreClient *firestore.Client, sendgridClient *sendgrid.Client, recheckPeriod time.Duration) *Poller {
	return &Poller{
		firestoreClient: firestoreClient,
		sendgridClient:  sendgridClient,
		recheckPeriod:   recheckPeriod,
	}
}

func (p *Poller) Run(ctx context.Context) error {
	ticker := time.NewTicker(p.recheckPeriod)
	defer ticker.Stop()

	// Poll once right away --- ticker doesn't fire until the tick period has
	// elapsed.
	if err := p.pollPatients(ctx); err != nil {
		slog.ErrorContext(ctx, "Error during poller pass", slog.Any("err", err))
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		if err := p.pollPatients(ctx); err != nil {
			slog.ErrorContext(ctx, "Error during poller pass", slog.Any("err", err))
		}
	}
}

func (p *Poller) pollPatients(ctx context.Context) error {
	slog.InfoContext(ctx, "Starting poller pass")
	defer func() {
		slog.InfoContext(ctx, "Finished poller pass")
	}()

	patientsCollection := p.firestoreClient.Collection("Patients")
	patientsIter := patientsCollection.DocumentRefs(ctx)
	for {
		patientDocRef, err := patientsIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("while iterating patients: %w", err)
		}

		slog.InfoContext(ctx, "Polling medications for patient", slog.String("patient", patientDocRef.ID))

		if err := p.processPatient(ctx, patientDocRef); err != nil {
			return fmt.Errorf("while polling medications for patient %s: %w", patientDocRef.ID, err)
		}
	}

	return nil
}

func (p *Poller) processPatient(ctx context.Context, patientDocRef *firestore.DocumentRef) error {
	var medicationAlert *MedicationAlert

	err := p.firestoreClient.RunTransaction(ctx, func(ctx context.Context, txn *firestore.Transaction) error {
		now := time.Now()

		patientDocSnap, err := txn.Get(patientDocRef)
		if err != nil {
			return fmt.Errorf("while reading patient: %w", err)
		}

		patient := &dbtypes.Patient{}
		if err := patientDocSnap.DataTo(patient); err != nil {
			return fmt.Errorf("while deserializing patient: %w", err)
		}

		// Remember that the transaction function can be executed multiple
		// times, so it's important that we initialize the medication alert from
		// scratch each time.
		medicationAlert = &MedicationAlert{
			NotifyUsers:     patient.ManagingUsers,
			ShowPatientLink: webui.ShowPersonLink(patient.ID),
		}

		for _, medication := range patient.Medications {
			// Send prescription length warnings.
			durationSinceLastFilled := now.Sub(medication.PrescriptionLastFilledAt)
			fiveDayWarningDuration := time.Duration(medication.PrescriptionLengthDays-5) * 24 * time.Hour
			twoDayWarningDuration := time.Duration(medication.PrescriptionLengthDays-2) * 24 * time.Hour
			if durationSinceLastFilled >= fiveDayWarningDuration && !medication.Prescription5DayWarningSent {
				medicationAlert.PrescriptionLengthAlerts = append(medicationAlert.PrescriptionLengthAlerts, PrescriptionLengthAlert{
					Name:                     medication.Name,
					Info:                     "5 day warning",
					PrescriptionLastFilledAt: medication.PrescriptionLastFilledAt,
					PrescriptionLengthDays:   medication.PrescriptionLengthDays,
				})
				medication.Prescription5DayWarningSent = true
			}
			if durationSinceLastFilled >= twoDayWarningDuration && !medication.Prescription2DayWarningSent {
				medicationAlert.PrescriptionLengthAlerts = append(medicationAlert.PrescriptionLengthAlerts, PrescriptionLengthAlert{
					Name:                     medication.Name,
					Info:                     "2 day warning",
					PrescriptionLastFilledAt: medication.PrescriptionLastFilledAt,
					PrescriptionLengthDays:   medication.PrescriptionLengthDays,
				})
				medication.Prescription2DayWarningSent = true
			}
		}

		// Write back patient.
		if err := txn.Set(patientDocRef, patient); err != nil {
			return fmt.Errorf("while updating firestore: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("while executing transaction: %w", err)
	}

	slog.InfoContext(ctx, "Sending medication alert", slog.Any("alert", medicationAlert))
	if err := p.sendAlert(ctx, medicationAlert); err != nil {
		return fmt.Errorf("while sending medication alert: %w", err)
	}

	return nil
}

func (p *Poller) sendAlert(ctx context.Context, alert *MedicationAlert) error {
	if len(alert.PrescriptionLengthAlerts) == 0 {
		return nil
	}

	for _, userID := range alert.NotifyUsers {
		userSnap, err := p.firestoreClient.Collection("Users").Doc(userID).Get(ctx)
		if err != nil {
			return fmt.Errorf("while retrieving user %s: %w", userID, err)
		}

		user := &dbtypes.User{}
		if err := userSnap.DataTo(user); err != nil {
			return fmt.Errorf("while unmarshaling user %s: %w", userID, err)
		}

		if user.Email != "" {
			if err := p.sendEmailAlert(ctx, user, alert); err != nil {
				return fmt.Errorf("while sending email alert: %w", err)
			}
		}
	}

	return nil
}

const emailPlain = `
{{- if .PrescriptionLengthAlerts -}}
The following prescriptions are ending soon:
{{range .PrescriptionLengthAlerts -}}
* {{.Name}}: {{.Info}}.  Last filled on {{.PrescriptionLastFilledAt}} for {{.PrescriptionLengthDays}} days.
{{end}}

Manage in the Web UI: https://medtracker.dev{{.ShowPatientLink}}
{{end}}
`

var emailPlainTemplate = template.Must(template.New("email").Parse(emailPlain))

func (p *Poller) sendEmailAlert(ctx context.Context, user *dbtypes.User, alert *MedicationAlert) error {
	message := mail.NewV3Mail()
	message.From = mail.NewEmail("MedTracker Bot", "bot@medtracker.dev")
	message.Subject = "Medtracker Alert"

	personalization := mail.NewPersonalization()
	personalization.To = append(personalization.To, mail.NewEmail("", user.Email))
	message.Personalizations = append(message.Personalizations, personalization)

	textContent := &bytes.Buffer{}
	if err := emailPlainTemplate.Execute(textContent, alert); err != nil {
		return fmt.Errorf("while templating plain-text email content: %w", err)
	}

	message.Content = append(message.Content, mail.NewContent("text/plain", textContent.String()))

	resp, err := p.sendgridClient.SendWithContext(ctx, message)
	if err != nil {
		return fmt.Errorf("while sending mail through SendGrid: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("non-2XX response while sending mail through Sendgrid: %d %s", resp.StatusCode, resp.Body)
	}

	return nil
}
