package dbtypes

import (
	"time"

	"cloud.google.com/go/firestore"
)

// User represents a person registered and interacting with the application.
type User struct {
	ID    string `firestore:"id"`
	Email string `firestore:"email"`
}

// Session represents a log-in session for a User.
type Session struct {
	Cookie  string                 `firestore:"cookie"`
	User    *firestore.DocumentRef `firestore:"user"`
	Expires time.Time              `firestore:"expires"`
}

type Collection struct {
	ID            string   `firestore:"id"`
	DisplayName   string   `firestore:"displayName"`
	ManagingUsers []string `firestore:"managingUsers"`
}

// Images are hierarchically filed under a collection
type Image struct {
	ID          string   `firestore:"id"`
	Name        string   `firestore:"name"`
	StoragePath string   `firestore:"storagePath"`
	Tags        []string `firestore:"tags"`
}
