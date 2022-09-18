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
	m.HandleFunc("/show-patient", u.showPatientHandler)
	m.HandleFunc("/record-medication-refill", u.recordMedicationRefillHandler)
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

		params.Patients = append(params.Patients, uitemplates.ListPatientsPatient{
			DisplayName:     dbPatient.DisplayName,
			ShowPatientLink: ShowPatientLink(dbPatient.ID),
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

func ShowPatientLink(id string) string {
	q := url.Values{}
	q.Add("id", id)
	showPatientLink := &url.URL{
		Path:     "/show-patient",
		RawQuery: q.Encode(),
	}
	return showPatientLink.String()
}

func (u *WebUI) showPatientHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/show-patient" {
		glog.Errorf("Returning Not Found because showPatientHandler doesn't support path %q", r.URL.Path)
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

	if err := r.ParseForm(); err != nil {
		glog.Errorf("Error while parsing form: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	patientID := r.Form.Get("id")

	patientDocRef := u.firestoreClient.Collection("Patients").Doc(patientID)
	patientDocSnap, err := patientDocRef.Get(ctx)

	patient := &dbtypes.Patient{}
	if err := patientDocSnap.DataTo(patient); err != nil {
		glog.Errorf("Error while unmarshaling patient: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	// Permissions check --- is the user allowed to access this patient?
	allowed := false
	for _, mu := range patient.ManagingUsers {
		if mu == user.ID {
			allowed = true
		}
	}
	if !allowed {
		glog.Errorf("User %s is not allowed to view patient %s", user.ID, patient.ID)
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	params := &uitemplates.ShowPatientParams{}
	params.DisplayName = patient.DisplayName
	params.SelfLink = ShowPatientLink(patient.ID)
	for _, dbMed := range patient.Medications {

		expiry := dbMed.PrescriptionLastFilledAt.Add(time.Duration(dbMed.PrescriptionLengthDays) * 24 * time.Hour)
		remaining := expiry.Sub(time.Now())
		remainingDays := remaining.Truncate(time.Hour).Nanoseconds() / 1000 / 1000 / 1000 / 86400

		uiMed := &uitemplates.ShowPatientMedication{
			DisplayName:              dbMed.Name,
			RecordRefillLink:         recordMedicationRefillLink(patient.ID, dbMed.Name, ""),
			PrescriptionDaysLeft:     fmt.Sprintf("%d day(s)", remainingDays),
			PrescriptionLengthDays:   fmt.Sprintf("%d day(s)", dbMed.PrescriptionLengthDays),
			PrescriptionLastFilledOn: dbMed.PrescriptionLastFilledAt.Format("2006-01-02"),
		}
		params.Medications = append(params.Medications, uiMed)
	}

	content := bytes.Buffer{}
	if err := uitemplates.ShowPatientTemplate.Execute(&content, params); err != nil {
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

func recordMedicationRefillLink(patientID, medicationName, userError string) string {
	q := url.Values{}
	q.Add("patient-id", patientID)
	q.Add("medication-name", medicationName)
	if userError != "" {
		q.Add("user-error", userError)
	}
	showPatientLink := &url.URL{
		Path:     "/record-medication-refill",
		RawQuery: q.Encode(),
	}
	return showPatientLink.String()
}

func (u *WebUI) recordMedicationRefillHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/record-medication-refill" {
		glog.Errorf("Returning Not Found because recordMedicationRefillHandler doesn't support path %q", r.URL.Path)
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		u.recordMedicationRefillGetHandler(w, r)
		return
	case http.MethodPost:
		u.recordMedicationRefillPostHandler(w, r)
	default:
		glog.Errorf("Returning Bad Request because recordMedicationRefillHandler doesn't support path %q", r.URL.Path)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
}

func (u *WebUI) recordMedicationRefillGetHandler(w http.ResponseWriter, r *http.Request) {
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

	if err := r.ParseForm(); err != nil {
		glog.Errorf("Error while parsing form: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	patientID := r.Form.Get("patient-id")

	patientDocRef := u.firestoreClient.Collection("Patients").Doc(patientID)
	patientDocSnap, err := patientDocRef.Get(ctx)
	if err != nil {
		glog.Errorf("Errow while retrieving patient: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	patient := &dbtypes.Patient{}
	if err := patientDocSnap.DataTo(patient); err != nil {
		glog.Errorf("Error while unmarshaling patient: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	// Permissions check --- is the user allowed to access this patient?
	allowed := false
	for _, mu := range patient.ManagingUsers {
		if mu == user.ID {
			allowed = true
		}
	}
	if !allowed {
		glog.Errorf("User %s is not allowed to access patient %s", user.ID, patient.ID)
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	params := &uitemplates.RecordMedicationRefillParams{
		PatientID:          patient.ID,
		MedicationName:     r.Form.Get("medication-name"),
		PatientDisplayName: patient.DisplayName,
		SelfLink:           recordMedicationRefillLink(patient.ID, r.Form.Get("medication-name"), ""),
		UserError:          r.Form.Get("user-error"),
	}

	content := bytes.Buffer{}
	if err := uitemplates.RecordMedicationRefillTemplate.Execute(&content, params); err != nil {
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

func (u *WebUI) recordMedicationRefillPostHandler(w http.ResponseWriter, r *http.Request) {
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

	if err := r.ParseForm(); err != nil {
		glog.Errorf("Error while parsing form: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	patientID := r.Form.Get("patient-id")

	patientDocRef := u.firestoreClient.Collection("Patients").Doc(patientID)
	patientDocSnap, err := patientDocRef.Get(ctx)
	if err != nil {
		glog.Errorf("Errow while retrieving patient: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	patient := &dbtypes.Patient{}
	if err := patientDocSnap.DataTo(patient); err != nil {
		glog.Errorf("Error while unmarshaling patient: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	// Permissions check --- is the user allowed to access this patient?
	allowed := false
	for _, mu := range patient.ManagingUsers {
		if mu == user.ID {
			allowed = true
		}
	}
	if !allowed {
		glog.Errorf("User %s is not allowed to access patient %s", user.ID, patient.ID)
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	userErr, err := u.doRecordMedicationRefill(ctx, patient.ID, r.Form.Get("medication-name"), r.Form.Get("refill-date"))
	if err != nil {
		glog.Errorf("Error while recording medication refill: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if userErr != "" {
		http.Redirect(w, r, recordMedicationRefillLink(patientID, r.Form.Get("medication-name"), userErr), http.StatusFound)
		return
	}

	http.Redirect(w, r, ShowPatientLink(patientID), http.StatusFound)
}

func (u *WebUI) doRecordMedicationRefill(ctx context.Context, patientID, medicationName, refillDate string) (string, error) {
	refillTime, err := time.Parse("2006-01-02", refillDate)
	if err != nil {
		return fmt.Sprintf("Could not parse date %q", refillDate), nil
	}

	patientDocRef := u.firestoreClient.Collection("Patients").Doc(patientID)
	patientDocSnap, err := patientDocRef.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("while retrieving patient %s: %w", patientID, err)
	}

	patient := &dbtypes.Patient{}
	if err := patientDocSnap.DataTo(patient); err != nil {
		return "", fmt.Errorf("while unmarshaling patient %s: %w", patientID, err)
	}

	foundMed := false
	for _, med := range patient.Medications {
		if med.Name == medicationName {
			foundMed = true
			med.PrescriptionLastFilledAt = refillTime
			med.Prescription2DayWarningSent = false
			med.Prescription5DayWarningSent = false
		}
	}

	if !foundMed {
		return "No medication by that name", nil
	}

	_, err = patientDocRef.Update(ctx, []firestore.Update{{Path: "medications", Value: patient.Medications}}, firestore.LastUpdateTime(patientDocSnap.UpdateTime))
	if err != nil {
		return "", fmt.Errorf("while updating patient: %w", err)
	}

	return "", nil
}
