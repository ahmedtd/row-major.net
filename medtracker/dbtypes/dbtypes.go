package dbtypes

import (
	"time"

	"cloud.google.com/go/firestore"
)

// User represents a person registered and interacting with the application.
//
// One user can manage multiple patients, and a patient can be managed by
// multiple users.
type User struct {
	ID           string `firestore:"id"`
	Email        string `firestore:"email"`
	PasswordHash string `firestore:"passwordHash"`
}

// Session represents a log-in session for a User.
type Session struct {
	Cookie  string                 `firestore:"cookie"`
	User    *firestore.DocumentRef `firestore:"user"`
	Expires time.Time              `firestore:"expires"`
}

type Patient struct {
	ID string `firestore:"id"`

	DisplayName string `firestore:"displayName"`

	// The set of users who can manage this patient.
	ManagingUsers []string `firestore:"managingUsers"`

	Medications []*Medication `firestore:"medications"`
}

type Medication struct {
	Name string `firestore:"name"`

	// The current count of stock.
	//
	// For split pills, track the count of half-pills.
	StockCount int64 `firestore:"stockCount"`

	// A display name for the unit of medicine.  "Pill", "Half-pill", etc.
	StockUnit string `firestore:"stockUnit"`

	StockDecrementCount  int64  `firestore:"stockDecrementCount"`
	StockDecrementPeriod string `firestore:"stockDecrementPeriod"`

	RunwayAlertThreshold string `firestore:"runwayAlertThreshold"`

	NextStockDecrementAt time.Time `firestore:"nextStockDecrementAt"`

	PrescriptionLastFilledAt    time.Time `firestore:"prescriptionLastFilledAt"`
	PrescriptionLengthDays      int64     `firestore:"prescriptionLengthDays"`
	Prescription5DayWarningSent bool      `firestore:"prescription5DayWarningSent"`
	Prescription2DayWarningSent bool      `firestore:"prescription2DayWarningSent"`
}
