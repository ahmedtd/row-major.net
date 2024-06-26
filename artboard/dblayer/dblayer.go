package dblayer

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"row-major/artboard/dbtypes"

	"cloud.google.com/go/firestore"
	"github.com/golang/glog"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/iterator"
)

type DB struct {
	firestoreClient     *firestore.Client
	googleOAuthClientID string
}

func New(firestoreClient *firestore.Client, googleOAuthClientID string) *DB {
	return &DB{
		firestoreClient:     firestoreClient,
		googleOAuthClientID: googleOAuthClientID,
	}
}

var (
	ErrEmailMustNotBeEmpty        = errors.New("email must not be empty")
	ErrPasswordMustNotBeEmpty     = errors.New("password must not be empty")
	ErrUnknownUserOrWrongPassword = errors.New("unknown user or wrong password")
	ErrUserNotLoggedIn            = errors.New("user is not logged in")
	ErrPermissionDenied           = errors.New("permission denied")
)

// SessionFromGoogleFederation signs in a user based on a Google identity token
// returned from the "Sign in with Google" process.
func (db *DB) SessionFromGoogleFederation(ctx context.Context, idToken string) (*dbtypes.Session, error) {
	payload, err := idtoken.Validate(ctx, idToken, db.googleOAuthClientID)
	if err != nil {
		return nil, fmt.Errorf("while validating ID token: %w", err)
	}

	email := payload.Claims["email"]
	// displayName := payload.Claims["name"]
	// picture := payload.Claims["picture"]

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

	// TODO: Autocreate user?  Populate display name and profile picture?
	if userSnapshot == nil {
		return nil, ErrUnknownUserOrWrongPassword
	}

	// TODO: Mark user as a "Sign In With Google" user and deactivate their password?

	// Now we've found the user.  We know they authenticated successfully with
	// Google, so it's time to create their session.

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
		glog.Infof("No logged-in user because there was no session object corresponding to the cookie in the database.")
		return nil, nil
	}

	session := &dbtypes.Session{}
	if err := sessionSnapshot.DataTo(session); err != nil {
		return nil, fmt.Errorf("while unmarshaling session: %w", err)
	}

	if session.Expires.Before(time.Now()) {
		// Session object is expired; user is not logged in.
		glog.Infof("No logged-in user because the session object in the database was expired.")
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
