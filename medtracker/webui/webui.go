package webui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"row-major/medtracker/dblayer"
	"row-major/medtracker/dbtypes"
	"row-major/medtracker/webui/uitemplates"

	"cloud.google.com/go/firestore"
	"github.com/golang/glog"
	"google.golang.org/api/iterator"
)

type WebUI struct {
	firestoreClient     *firestore.Client
	db                  *dblayer.DB
	googleOAuthClientID string
}

func New(firestoreClient *firestore.Client, db *dblayer.DB, googleOAuthClientID string) *WebUI {
	return &WebUI{
		firestoreClient:     firestoreClient,
		db:                  db,
		googleOAuthClientID: googleOAuthClientID,
	}
}

func (u *WebUI) Register(m *http.ServeMux) {
	m.HandleFunc("/", u.homeHandler)
	m.HandleFunc("/log-in", u.logInHandler)
	m.HandleFunc("/log-out", u.logOutHandler)
	m.HandleFunc("/sign-in-with-google", u.signInWithGoogleHandler)
	m.HandleFunc("/list-patients", u.listPatientsHandler)
	m.HandleFunc("/create-person", u.createPersonHandler)
	m.HandleFunc("/show-patient", u.showPatientHandler)
	m.HandleFunc("/record-medication-refill", u.recordMedicationRefillHandler)
	m.HandleFunc("/create-medication", u.createMedicationHandler)
}

// getLoggedInUser loads the user associated with the session cookie in the
// request, if it exists.
func (u *WebUI) getLoggedInUser(ctx context.Context, r *http.Request) (string, *dbtypes.User, error) {
	var sessionCookie *http.Cookie
	for _, cookie := range r.Cookies() {
		if cookie.Name == "MedTracker-Session" {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil {
		// No session cookie; user is not logged in.
		glog.Infof("No logged-in user because there was no session cookie.")
		return "", nil, nil
	}

	user, err := u.db.UserFromSessionCookie(ctx, sessionCookie.Value)
	if err != nil {
		return "", nil, fmt.Errorf("while getting user from session cookie: %w", err)
	}

	return sessionCookie.Value, user, nil
}

func (u *WebUI) checkSession(ctx context.Context, w http.ResponseWriter, r *http.Request, redirectAfterLogin string) *dbtypes.User {
	var sessionCookie *http.Cookie
	for _, cookie := range r.Cookies() {
		if cookie.Name == "MedTracker-Session" {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil {
		// User is not logged in.  Send them to log in.
		glog.Infof("No logged-in user because there was no session cookie.  Redirecting to login.")
		http.Redirect(w, r, logInLink("", redirectAfterLogin), http.StatusFound)
		return nil
	}

	user, err := u.db.UserFromSessionCookie(ctx, sessionCookie.Value)
	if err != nil {
		glog.Infof("Error while validating session cookie: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return nil
	}
	if user == nil {
		// User is not logged in.  For example, there was a session cookie, but
		// it corresponds to an expired session.
		glog.Infof("Session cookie didn't correspond to an active session.")
		http.Redirect(w, r, logInLink("", redirectAfterLogin), http.StatusFound)
		return nil
	}

	return user
}

func (u *WebUI) checkUserAllowedToManagePatient(ctx context.Context, w http.ResponseWriter, r *http.Request, user *dbtypes.User, patientID string) bool {
	err := u.db.CheckUserAllowedToManagePatient(ctx, user, patientID)
	if errors.Is(err, dblayer.ErrPermissionDenied) {
		glog.Errorf("User is not allowed to view patient %s", patientID)
		http.Error(w, "Not Found", http.StatusNotFound)
		return false
	} else if err != nil {
		glog.Errorf("Error while checking that session is allowed to manage patient: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return false
	}

	return true
}

// homeHandler renders the home page.
func (u *WebUI) homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	params := &uitemplates.HomeParams{}

	_, user, err := u.getLoggedInUser(r.Context(), r)
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

func logInLink(userError, redirectTarget string) string {
	q := url.Values{}
	if userError != "" {
		q.Add("user-error", userError)
	}
	if redirectTarget != "" {
		q.Add("redirect-target", redirectTarget)
	}
	link := &url.URL{
		Path:     "/log-in",
		RawQuery: q.Encode(),
	}
	return link.String()
}

// logInHandler renders the login page.
func (u *WebUI) logInHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/log-in" {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		u.logInGetHandler(w, r)
		return
	case http.MethodPost:
		u.logInPostHandler(w, r)
		return
	default:
		glog.Errorf("Returning Bad Request because logInHandler doesn't support path %q", r.URL.Path)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
}

func signInWithGoogleTarget(redirectTarget string) string {
	q := url.Values{}
	if redirectTarget != "" {
		q.Add("redirect-target", redirectTarget)
	}
	link := &url.URL{
		Path:     "/sign-in-with-google",
		RawQuery: q.Encode(),
	}
	return link.String()
}

func (u *WebUI) logInGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, user, err := u.getLoggedInUser(ctx, r)
	if err != nil {
		glog.Errorf("Error while getting logged-in user: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if err := r.ParseForm(); err != nil {
		glog.Errorf("Error while parsing form: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if user != nil {
		// User is already logged in.
		if target := r.Form.Get("redirect-target"); target != "" {
			http.Redirect(w, r, target, http.StatusFound)
			return
		}
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	params := &uitemplates.LogInParams{
		UserError:            r.Form.Get("user-error"),
		GoogleOAuthClientID:  u.googleOAuthClientID,
		SignInWithGoogleLink: signInWithGoogleTarget(r.Form.Get("redirect-target")),
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
}

func (u *WebUI) logInPostHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, user, err := u.getLoggedInUser(ctx, r)
	if err != nil {
		glog.Errorf("Error while getting logged-in user: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if err := r.ParseForm(); err != nil {
		glog.Errorf("Error while parsing form: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if user != nil {
		// User is already logged in.
		if target := r.Form.Get("redirect-target"); target != "" {
			http.Redirect(w, r, target, http.StatusFound)
			return
		}
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	session, err := u.db.SessionFromPassword(ctx, r.PostForm.Get("email"), r.PostForm.Get("password"))
	if err == dblayer.ErrEmailMustNotBeEmpty {
		http.Redirect(w, r, logInLink("Email must not be empty", r.Form.Get("redirect-target")), http.StatusFound)
		return
	}
	if err == dblayer.ErrPasswordMustNotBeEmpty {
		http.Redirect(w, r, logInLink("Password must not be empty", r.Form.Get("redirect-target")), http.StatusFound)
		return
	}
	if err == dblayer.ErrUnknownUserOrWrongPassword {
		http.Redirect(w, r, logInLink("Unknown user or wrong password", r.Form.Get("redirect-target")), http.StatusFound)
		return
	}
	if err != nil {
		glog.Errorf("Error while processing log in form: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	cookie := &http.Cookie{
		Name:     "MedTracker-Session",
		Value:    session.Cookie,
		SameSite: http.SameSiteStrictMode,
		Expires:  session.Expires,
	}

	// User successfully logged in
	http.SetCookie(w, cookie)

	target := r.Form.Get("redirect-target")
	if target == "" {
		target = "/"
	}

	http.Redirect(w, r, target, http.StatusFound)
}

func (u *WebUI) logOutHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/log-out" {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		u.logOutGetHandler(w, r)
		return
	case http.MethodPost:
		u.logOutPostHandler(w, r)
		return
	}
}

func (u *WebUI) logOutGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, user, err := u.getLoggedInUser(ctx, r)
	if err != nil {
		glog.Errorf("Error while getting logged-in user: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if user == nil {
		// User is already logged out?
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	content, err := uitemplates.LogOutPage(&uitemplates.LogOutParams{})
	if err != nil {
		glog.Errorf("Error while executing template: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(content); err != nil {
		// It's too late to write an error to the HTTP response.
		glog.Errorf("Error while writing output: %v", err)
		return
	}
}

func (u *WebUI) logOutPostHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	cookie, user, err := u.getLoggedInUser(ctx, r)
	if err != nil {
		glog.Errorf("Error while getting logged-in user: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if user == nil {
		// User is already logged out.
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	err = u.db.DeleteSession(ctx, cookie)
	if err != nil {
		glog.Errorf("Error while deleting session: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:   "MedTracker-Session",
		MaxAge: -1,
	})

	http.Redirect(w, r, "/", http.StatusFound)
}

// signInWithGoogleHandler accepts the "Sign In With Google" ID token POST.
func (u *WebUI) signInWithGoogleHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/sign-in-with-google" {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodPost:
		u.signInWithGooglePostHandler(w, r)
		return
	default:
		glog.Errorf("Returning Bad Request because signInWithGoogleHandler doesn't support path %q", r.URL.Path)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
}

func (u *WebUI) signInWithGooglePostHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		glog.Errorf("Error while parsing form: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	session, err := u.db.SessionFromGoogleFederation(ctx, r.PostForm.Get("credential"))
	if err == dblayer.ErrUnknownUserOrWrongPassword {
		http.Redirect(w, r, logInLink("Unknown user or wrong password", r.Form.Get("redirect-target")), http.StatusFound)
		return
	}
	if err != nil {
		glog.Errorf("Error while processing log in form: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	cookie := &http.Cookie{
		Name:     "MedTracker-Session",
		Value:    session.Cookie,
		SameSite: http.SameSiteStrictMode,
		Expires:  session.Expires,
	}

	// User successfully logged in
	http.SetCookie(w, cookie)

	target := r.Form.Get("redirect-target")
	if target == "" {
		target = "/"
	}

	http.Redirect(w, r, target, http.StatusFound)
}

// patientsHandler renders the /patients list for the logged-in user.
func (u *WebUI) listPatientsHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/list-patients" {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	ctx := r.Context()

	user := u.checkSession(ctx, w, r, "/list-patients")
	if user == nil {
		// checkSession already wrote an error or redirect
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
			glog.Errorf("Error while iterating patients managed by user: %v", err)
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

func createPersonLink(userError string) string {
	q := url.Values{}
	if userError != "" {
		q.Add("user-error", userError)
	}
	u := &url.URL{
		Path:     "/create-person",
		RawQuery: q.Encode(),
	}
	return u.String()
}

func (u *WebUI) createPersonHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/create-person" {
		glog.Errorf("Returning Not Found because createPersonHandler doesn't support path %q", r.URL.Path)
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		u.createPersonGetHandler(w, r)
		return
	case http.MethodPost:
		u.createPersonPostHandler(w, r)
		return
	default:
		glog.Errorf("Returning Bad Request because recordMedicationRefillHandler doesn't support path %q", r.URL.Path)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
}

func (u *WebUI) createPersonGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		glog.Errorf("Error while parsing form: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
	}

	user := u.checkSession(ctx, w, r, createPersonLink(r.Form.Get("user-error")))
	if user == nil {
		// checkSession already wrote an error redirect.
	}
	// No permissions check necessary.

	params := &uitemplates.CreatePersonParams{
		UserError: r.Form.Get("user-error"),
	}
	content, err := uitemplates.CreatePersonPage(params)
	if err != nil {
		glog.Errorf("Error while executing template: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(content); err != nil {
		// It's too late to write an error to the HTTP response.
		glog.Errorf("Error while writing output: %v", err)
		return
	}
}

func (u *WebUI) createPersonPostHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		glog.Errorf("Error while parsing form: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	user := u.checkSession(ctx, w, r, createPersonLink(r.Form.Get("user-error")))
	if user == nil {
		// checkSession already wrote an error or redirect.
		return
	}
	// No permissions check.

	person := &dbtypes.Patient{
		DisplayName:   r.Form.Get("name"),
		ManagingUsers: []string{user.ID},
	}

	err := u.db.CreatePerson(ctx, person)
	if err != nil {
		glog.Errorf("Error while creating person: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/list-patients", http.StatusFound)
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

	if err := r.ParseForm(); err != nil {
		glog.Errorf("Error while parsing form: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	patientID := r.Form.Get("id")

	user := u.checkSession(ctx, w, r, ShowPatientLink(patientID))
	if user == nil {
		// checkSession already wrote an error or redirect
		return
	}
	if !u.checkUserAllowedToManagePatient(ctx, w, r, user, patientID) {
		// The permissions check has already written a response.
		return
	}

	patientDocRef := u.firestoreClient.Collection("Patients").Doc(patientID)
	patientDocSnap, err := patientDocRef.Get(ctx)
	if err != nil {
		glog.Errorf("Error while getting patient: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	patient := &dbtypes.Patient{}
	if err := patientDocSnap.DataTo(patient); err != nil {
		glog.Errorf("Error while unmarshaling patient: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	params := &uitemplates.ShowPatientParams{
		DisplayName:          patient.DisplayName,
		CreateMedicationLink: createMedicationLink(patient.ID, ""),
		SelfLink:             ShowPatientLink(patient.ID),
	}
	for _, dbMed := range patient.Medications {
		expiry := dbMed.PrescriptionLastFilledAt.Add(time.Duration(dbMed.PrescriptionLengthDays) * 24 * time.Hour)
		remaining := time.Until(expiry)
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
		return
	default:
		glog.Errorf("Returning Bad Request because recordMedicationRefillHandler doesn't support path %q", r.URL.Path)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
}

func (u *WebUI) recordMedicationRefillGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		glog.Errorf("Error while parsing form: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	patientID := r.Form.Get("patient-id")
	medicationName := r.Form.Get("medication-name")

	user := u.checkSession(ctx, w, r, recordMedicationRefillLink(patientID, medicationName, ""))
	if user == nil {
		// checkSession already wrote an error or redirect
		return
	}
	if !u.checkUserAllowedToManagePatient(ctx, w, r, user, patientID) {
		// The permissions check has already written a response.
		return
	}

	patient, err := u.db.GetPatient(ctx, patientID)
	if err != nil {
		glog.Errorf("Error while retrieving patient: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
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

	if err := r.ParseForm(); err != nil {
		glog.Errorf("Error while parsing form: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	patientID := r.Form.Get("patient-id")
	medicationName := r.Form.Get("medication-name")

	user := u.checkSession(ctx, w, r, recordMedicationRefillLink(patientID, medicationName, ""))
	if user == nil {
		// checkSession already wrote an error or redirect
		return
	}
	if !u.checkUserAllowedToManagePatient(ctx, w, r, user, patientID) {
		// The permissions check has already written a response.
		return
	}

	err := u.db.RecordMedicationRefill(ctx, patientID, r.Form.Get("medication-name"), r.Form.Get("refill-date"))
	if errors.Is(err, dblayer.ErrCouldNotParseDate) {
		http.Redirect(w, r, recordMedicationRefillLink(patientID, r.Form.Get("medication-name"), "Could not parse date"), http.StatusFound)
		return
	} else if errors.Is(err, dblayer.ErrMedicationNotFound) {
		http.Redirect(w, r, recordMedicationRefillLink(patientID, r.Form.Get("medication-name"), "Medication not found"), http.StatusFound)
		return
	}
	if err != nil {
		glog.Errorf("Error while recording medication refill: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, ShowPatientLink(patientID), http.StatusFound)
}

func createMedicationLink(patientID, userError string) string {
	q := url.Values{}
	q.Add("patient-id", patientID)
	if userError != "" {
		q.Add("user-error", userError)
	}
	link := &url.URL{
		Path:     "/create-medication",
		RawQuery: q.Encode(),
	}
	return link.String()
}

func (u *WebUI) createMedicationHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/create-medication" {
		glog.Errorf("Returning Not Found because createMedicationHandler doesn't support path %q", r.URL.Path)
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		u.createMedicationGetHandler(w, r)
		return
	case http.MethodPost:
		u.createMedicationPostHandler(w, r)
		return
	default:
		glog.Errorf("Returning Bad Request because createMedicationHandler doesn't support path %q", r.URL.Path)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
}

func (u *WebUI) createMedicationGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		glog.Errorf("Error while parsing form: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	patientID := r.Form.Get("patient-id")

	user := u.checkSession(ctx, w, r, createMedicationLink(patientID, ""))
	if user == nil {
		// checkSession already wrote an error or redirect
		return
	}
	if !u.checkUserAllowedToManagePatient(ctx, w, r, user, patientID) {
		// The permissions check has already written a response.
		return
	}

	patient, err := u.db.GetPatient(ctx, patientID)
	if err != nil {
		glog.Errorf("Error while retrieving patient: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	params := &uitemplates.CreateMedicationParams{
		PatientID:          patient.ID,
		PatientDisplayName: patient.DisplayName,
		SelfLink:           createMedicationLink(patientID, ""),
		UserError:          r.Form.Get("user-error"),
	}

	content := bytes.Buffer{}
	if err := uitemplates.CreateMedicationTemplate.Execute(&content, params); err != nil {
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

func (u *WebUI) createMedicationPostHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		glog.Errorf("Error while parsing form: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	patientID := r.Form.Get("patient-id")

	user := u.checkSession(ctx, w, r, createMedicationLink(patientID, ""))
	if user == nil {
		// checkSession already wrote an error or redirect
		return
	}
	if !u.checkUserAllowedToManagePatient(ctx, w, r, user, patientID) {
		// The permissions check has already written a response.
		return
	}

	err := u.db.CreateMedication(ctx, patientID, r.Form.Get("medication-name"), r.Form.Get("rx-length-days"), r.Form.Get("rx-filled-at"))
	if errors.Is(err, dblayer.ErrCouldNotParseDate) {
		http.Redirect(w, r, createMedicationLink(patientID, "Could not parse date"), http.StatusFound)
		return
	} else if errors.Is(err, dblayer.ErrMedicationAlreadyExists) {
		http.Redirect(w, r, createMedicationLink(patientID, "Medication already exists"), http.StatusFound)
		return
	} else if errors.Is(err, dblayer.ErrCouldNotParsePrescriptionLength) {
		http.Redirect(w, r, createMedicationLink(patientID, "Could not parse prescription length"), http.StatusFound)
	} else if err != nil {
		glog.Errorf("Error while creating medication: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, ShowPatientLink(patientID), http.StatusFound)
}
