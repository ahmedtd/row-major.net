package webui

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"row-major/medtracker/dbtypes"
	"row-major/medtracker/webui/uitemplates"

	"cloud.google.com/go/firestore"
	"github.com/golang/glog"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/api/iterator"
)

type WebUI struct {
	firestoreClient *firestore.Client
}

func New(firestoreClient *firestore.Client) *WebUI {
	return &WebUI{
		firestoreClient: firestoreClient,
	}
}

func (u *WebUI) Register(m *http.ServeMux) {
	m.HandleFunc("/", u.homeHandler)
	m.HandleFunc("/log-in", u.logInHandler)
	m.HandleFunc("/list-patients", u.listPatientsHandler)
}

// getLoggedInUser loads the user associated with the session cookie in the
// request, if it exists.
func (u *WebUI) getLoggedInUser(ctx context.Context, r *http.Request) (*dbtypes.User, error) {
	var sessionCookie *http.Cookie
	for _, cookie := range r.Cookies() {
		if cookie.Name == "MedTracker-Session" {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil {
		// No session cookie; user is not logged in.
		glog.Infof("No logged-in user because there was no session cookie.")
		return nil, nil
	}

	var sessionSnapshot *firestore.DocumentSnapshot
	sessionIter := u.firestoreClient.Collection("Sessions").Where("cookie", "==", sessionCookie.Value).Documents(ctx)
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

// homeHandler renders the home page.
func (u *WebUI) homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	params := &uitemplates.HomeParams{}

	user, err := u.getLoggedInUser(r.Context(), r)
	if err != nil {
		glog.Errorf("Error while getting logged-in user: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}
	if user != nil {
		params.LoggedIn = true
	}

	content := bytes.Buffer{}
	if err := uitemplates.HomeTemplate.Execute(&content, params); err != nil {
		glog.Errorf("Error while executing template: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if _, err := io.Copy(w, &content); err != nil {
		// It's too late to write an error to the HTTP response.
		glog.Errorf("Error while writing output: %v", err)
		return
	}
}

func (u *WebUI) doLogIn(ctx context.Context, email, password string) (cookie *http.Cookie, toast string, err error) {
	if email == "" {
		return nil, "Email must not be empty", nil
	}

	if password == "" {
		return nil, "Password must not be empty", nil
	}

	var userSnapshot *firestore.DocumentSnapshot
	userIter := u.firestoreClient.Collection("Users").Where("email", "==", email).Documents(ctx)
	for {
		userSnapshot, err = userIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("while looking up user with email %q: %w", email, err)
		}

		// We only consider a single user.
		break
	}

	if userSnapshot == nil {
		return nil, "Unknown user or wrong password", nil
	}

	user := &dbtypes.User{}
	if err := userSnapshot.DataTo(user); err != nil {
		return nil, "", fmt.Errorf("while unmarshaling user %q: %w", email, err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, "Unknown user or wrong password", nil
	}

	sessionCookieBytes := make([]byte, 32)
	if _, err := rand.Read(sessionCookieBytes); err != nil {
		return nil, "", fmt.Errorf("while generating session cookie: %w", err)
	}

	sessionCookie := base64.StdEncoding.EncodeToString(sessionCookieBytes)

	expires := time.Now().Add(18 * time.Hour)

	sessions := u.firestoreClient.Collection("Sessions")
	_, _, err = sessions.Add(ctx, &dbtypes.Session{
		Cookie:  sessionCookie,
		User:    userSnapshot.Ref,
		Expires: expires,
	})
	if err != nil {
		return nil, "", fmt.Errorf("while storing session cookie: %w", err)
	}

	cookie = &http.Cookie{
		Name:     "MedTracker-Session",
		Value:    sessionCookie,
		SameSite: http.SameSiteStrictMode,
		Expires:  expires,
	}

	return cookie, "", nil
}

// logInHandler renders the login page.
func (u *WebUI) logInHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/log-in" {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	ctx := r.Context()

	user, err := u.getLoggedInUser(ctx, r)
	if err != nil {
		glog.Errorf("Error while getting logged-in user: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if user != nil {
		// User is already logged in.  Send them back home.
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if r.Method == http.MethodPost {
		// The user is submitting a login form.

		if err := r.ParseForm(); err != nil {
			glog.Errorf("Error while parsing form: %v", err)
			http.Error(w, "Internal Error", http.StatusInternalServerError)
			return
		}

		cookie, userErr, err := u.doLogIn(ctx, r.PostForm.Get("email"), r.PostForm.Get("password"))
		if err != nil {
			glog.Errorf("Error while processing log in form: %v", err)
			http.Error(w, "Internal Error", http.StatusInternalServerError)
			return
		}

		if userErr != "" {
			// Render log in form with user error.

			// TODO: Should we instead redirect back to the login form with the
			// user error as a query parameter?

			params := &uitemplates.LogInParams{
				UserError: userErr,
			}

			content := bytes.Buffer{}
			if err := uitemplates.LogInTemplate.Execute(&content, params); err != nil {
				glog.Errorf("Error while executing template: %v", err)
				http.Error(w, "Internal Error", http.StatusInternalServerError)
				return
			}

			if _, err := io.Copy(w, &content); err != nil {
				// It's too late to write an error to the HTTP response.
				glog.Errorf("Error while writing output: %v", err)
				return
			}

			return
		}

		// User successfully logged in
		http.SetCookie(w, cookie)
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	// Otherwise, render login form.

	params := &uitemplates.LogInParams{}

	content := bytes.Buffer{}
	if err := uitemplates.LogInTemplate.Execute(&content, params); err != nil {
		glog.Errorf("Error while executing template: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if _, err := io.Copy(w, &content); err != nil {
		// It's too late to write an error to the HTTP response.
		glog.Errorf("Error while writing output: %v", err)
		return
	}
}

// patientsHandler renders the /patients list for the logged-in user.
func (u *WebUI) listPatientsHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/list-patients" {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	ctx := r.Context()

	user, err := u.getLoggedInUser(ctx, r)
	if err != nil {
		glog.Errorf("Error while getting logged-in user: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if user == nil {
		// User is not logged in.  Send them to log in.
		//
		// TODO: Have log-in redirect back to this page?
		http.Redirect(w, r, "/log-in", http.StatusFound)
		return
	}

	params := &uitemplates.ListPatientsParams{}

	// TODO: Pull list of patients for this user.
	patientsIter := u.firestoreClient.Collection("Patients").Where("managingUsers", "array-contains", user.ID).Documents(ctx)
	for {
		patientSnapshot, err := patientsIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			glog.Errorf("Error while iterating patients managed by user %q: %v", user.Email, err)
			http.Error(w, "Internal Error", http.StatusInternalServerError)
			return
		}

		dbPatient := &dbtypes.Patient{}
		if err := patientSnapshot.DataTo(dbPatient); err != nil {
			glog.Errorf("Error while extracting patient %s: %v", patientSnapshot.Ref.ID, err)
			http.Error(w, "Internal Error", http.StatusInternalServerError)
			return
		}

		q := url.Values{}
		q.Add("patient-id", dbPatient.ID)
		showPatientLink := &url.URL{
			Path:     "/show-patient",
			RawQuery: q.Encode(),
		}

		showPatientLink.Query().Add("patient-id", dbPatient.ID)

		params.Patients = append(params.Patients, uitemplates.ListPatientsPatient{
			DisplayName:     dbPatient.DisplayName,
			ShowPatientLink: showPatientLink.String(),
		})
	}

	content := bytes.Buffer{}
	if err := uitemplates.ListPatientsTemplate.Execute(&content, params); err != nil {
		glog.Errorf("Error while executing template: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if _, err := io.Copy(w, &content); err != nil {
		// It's too late to write an error to the HTTP response.
		glog.Errorf("Error while writing output: %v", err)
		return
	}
}
