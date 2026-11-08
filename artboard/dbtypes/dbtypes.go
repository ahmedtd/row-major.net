package dbtypes

import (
	"time"

	"cloud.google.com/go/firestore"
)

// User represents a person registered and interacting with the application.
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

// Collection is a set of media backed by a Google Cloud Storage location.
type Collection struct {
	ID            string   `firestore:"id"`
	DisplayName   string   `firestore:"displayName"`
	ManagingUsers []string `firestore:"managingUsers"`

	// StorageURL is the canonical gs://bucket/prefix/ location holding the
	// collection's media.  The server accesses it using its own (Workload
	// Identity) credentials.
	StorageURL string `firestore:"storageURL"`
}

// Images are hierarchically filed under a collection
type Image struct {
	ID          string   `firestore:"id"`
	Name        string   `firestore:"name"`
	StoragePath string   `firestore:"storagePath"`
	Tags        []string `firestore:"tags"`
}
