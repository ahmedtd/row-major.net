package dblayer

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"row-major/artboard/dbtypes"

	"cloud.google.com/go/firestore"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type DB struct {
	firestoreClient *firestore.Client
}

func New(firestoreClient *firestore.Client) *DB {
	return &DB{
		firestoreClient: firestoreClient,
	}
}

var (
	ErrEmailMustNotBeEmpty        = errors.New("email must not be empty")
	ErrPasswordMustNotBeEmpty     = errors.New("password must not be empty")
	ErrUnknownUserOrWrongPassword = errors.New("unknown user or wrong password")
	ErrUserNotLoggedIn            = errors.New("user is not logged in")
	ErrPermissionDenied           = errors.New("permission denied")
)

// SessionFromPassword runs the password-based login process for a given user,
// returning a session ID or an error.
func (db *DB) SessionFromPassword(ctx context.Context, email, password string) (*dbtypes.Session, error) {
	if email == "" {
		return nil, ErrEmailMustNotBeEmpty
	}

	if password == "" {
		return nil, ErrPasswordMustNotBeEmpty
	}

	var userSnapshot *firestore.DocumentSnapshot
	userIter := db.firestoreClient.Collection("Users").Where("email", "==", email).Documents(ctx)
	defer userIter.Stop()
	for {
		var err error
		userSnapshot, err = userIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("while looking up user with email %q: %w", email, err)
		}

		// We only consider a single user.
		break
	}

	if userSnapshot == nil {
		return nil, ErrUnknownUserOrWrongPassword
	}

	user := &dbtypes.User{}
	if err := userSnapshot.DataTo(user); err != nil {
		return nil, fmt.Errorf("while unmarshaling user %q: %w", email, err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrUnknownUserOrWrongPassword
	}

	sessionCookieBytes := make([]byte, 32)
	if _, err := rand.Read(sessionCookieBytes); err != nil {
		return nil, fmt.Errorf("while generating session cookie: %w", err)
	}

	sessionCookie := base64.StdEncoding.EncodeToString(sessionCookieBytes)

	expires := time.Now().Add(18 * time.Hour)

	sessions := db.firestoreClient.Collection("Sessions")
	session := &dbtypes.Session{
		Cookie:  sessionCookie,
		User:    userSnapshot.Ref,
		Expires: expires,
	}
	if _, _, err := sessions.Add(ctx, session); err != nil {
		return nil, fmt.Errorf("while storing session cookie: %w", err)
	}

	return session, nil
}

// DeleteSession deletes a session by its cookie.
func (db *DB) DeleteSession(ctx context.Context, cookie string) error {
	sessionIter := db.firestoreClient.Collection("Sessions").Where("cookie", "==", cookie).Documents(ctx)
	defer sessionIter.Stop()
	for {
		sessionSnapshot, err := sessionIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("while looking up session: %w", err)
		}

		_, err = sessionSnapshot.Ref.Delete(ctx, firestore.LastUpdateTime(sessionSnapshot.UpdateTime))
		if err != nil {
			return fmt.Errorf("while deleting session: %w", err)
		}
	}

	return nil
}

// UserFromSessionCookie looks up a session from its cookie, and then returns
// the corresponding user.
func (db *DB) UserFromSessionCookie(ctx context.Context, cookie string) (*dbtypes.User, error) {
	var sessionSnapshot *firestore.DocumentSnapshot
	sessionIter := db.firestoreClient.Collection("Sessions").Where("cookie", "==", cookie).Documents(ctx)
	defer sessionIter.Stop()
	for {
		var err error
		sessionSnapshot, err = sessionIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("while looking up session: %w", err)
		}

		// We only consider a single session.
		break
	}
	if sessionSnapshot == nil {
		// Session object must have been cleaned up due to expiration; user is not logged in.
		slog.InfoContext(ctx, "No logged-in user because there was no session object corresponding to the cookie in the database.")
		return nil, nil
	}

	session := &dbtypes.Session{}
	if err := sessionSnapshot.DataTo(session); err != nil {
		return nil, fmt.Errorf("while unmarshaling session: %w", err)
	}

	if session.Expires.Before(time.Now()) {
		// Session object is expired; user is not logged in.
		slog.InfoContext(ctx, "No logged-in user because the session object in the database was expired.")
		return nil, nil
	}

	userSnapshot, err := session.User.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("while getting user linked from session: %w", err)
	}

	user := &dbtypes.User{}
	if err := userSnapshot.DataTo(user); err != nil {
		return nil, fmt.Errorf("while unmarshaling user: %w", err)
	}

	return user, nil
}

// CreateCollection stores a new collection, assigning its ID.
func (db *DB) CreateCollection(ctx context.Context, collection *dbtypes.Collection) error {
	ref := db.firestoreClient.Collection("Collections").NewDoc()
	collection.ID = ref.ID
	if _, err := ref.Create(ctx, collection); err != nil {
		return fmt.Errorf("while creating collection: %w", err)
	}
	return nil
}

// ListCollectionsForUser returns all collections that the user manages.
func (db *DB) ListCollectionsForUser(ctx context.Context, user *dbtypes.User) ([]*dbtypes.Collection, error) {
	iter := db.firestoreClient.Collection("Collections").Where("managingUsers", "array-contains", user.ID).Documents(ctx)
	defer iter.Stop()

	var collections []*dbtypes.Collection
	for {
		snap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("while iterating collections: %w", err)
		}

		collection := &dbtypes.Collection{}
		if err := snap.DataTo(collection); err != nil {
			return nil, fmt.Errorf("while unmarshaling collection %s: %w", snap.Ref.ID, err)
		}
		collections = append(collections, collection)
	}

	return collections, nil
}

// GetCollectionForUser retrieves a collection, returning ErrPermissionDenied if
// it does not exist or the user does not manage it.
func (db *DB) GetCollectionForUser(ctx context.Context, user *dbtypes.User, id string) (*dbtypes.Collection, error) {
	if id == "" {
		return nil, ErrPermissionDenied
	}

	snap, err := db.firestoreClient.Collection("Collections").Doc(id).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return nil, ErrPermissionDenied
	}
	if err != nil {
		return nil, fmt.Errorf("while retrieving collection %s: %w", id, err)
	}

	collection := &dbtypes.Collection{}
	if err := snap.DataTo(collection); err != nil {
		return nil, fmt.Errorf("while unmarshaling collection %s: %w", id, err)
	}

	if !slices.Contains(collection.ManagingUsers, user.ID) {
		return nil, ErrPermissionDenied
	}

	return collection, nil
}
