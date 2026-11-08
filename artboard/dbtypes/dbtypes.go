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

// ObjectMetadata holds application data (such as tags) about a single Cloud
// Storage object within a collection.  It is stored in the "ObjectMetadata"
// Firestore collection.
//
// Entries are created lazily, the first time there is something to record
// about an object, so most objects in a collection have no entry.
type ObjectMetadata struct {
	ID string `firestore:"id"`

	// Collection is the ID of the Collection the object belongs to.
	Collection string `firestore:"collection"`

	// Path is the object's name relative to the collection's storage prefix.
	Path string `firestore:"path"`

	Tags []string `firestore:"tags"`
}
