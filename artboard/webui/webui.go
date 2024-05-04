package webui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"row-major/artboard/dblayer"
	"row-major/artboard/dbtypes"
	"row-major/artboard/webui/uitemplates"
	glog "row-major/bazel-row-major.net/external/com_github_golang_glog"
)

type WebUI struct {
	db                  *dblayer.DB
	googleOAuthClientID string
}

func New(db *dblayer.DB, googleOAuthClientID string) *WebUI {
	return &WebUI{
		db:                  db,
		googleOAuthClientID: googleOAuthClientID,
	}
}

func (u *WebUI) Register(m *http.ServeMux) {
	m.HandleFunc("/", u.homeHandler)
	m.HandleFunc("/", u.homeHandler)
	m.HandleFunc("/log-in", u.logInHandler)
	m.HandleFunc("/log-out", u.logOutHandler)
	m.HandleFunc("/sign-in-with-google", u.signInWithGoogleHandler)
}

// getLoggedInUser loads the user associated with the session cookie in the
// request, if it exists.
func (u *WebUI) getLoggedInUser(ctx context.Context, r *http.Request) (string, *dbtypes.User, error) {
	var sessionCookie *http.Cookie
	for _, cookie := range r.Cookies() {
		if cookie.Name == "ArtBoard-Session" {
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
		if cookie.Name == "ArtBoard-Session" {
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
	// case http.MethodPost:
	// 	u.logInPostHandler(w, r)
	// 	return
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

// func (u *WebUI) logInPostHandler(w http.ResponseWriter, r *http.Request) {
// 	ctx := r.Context()

// 	_, user, err := u.getLoggedInUser(ctx, r)
// 	if err != nil {
// 		glog.Errorf("Error while getting logged-in user: %v", err)
// 		http.Error(w, "Internal Error", http.StatusInternalServerError)
// 		return
// 	}

// 	if err := r.ParseForm(); err != nil {
// 		glog.Errorf("Error while parsing form: %v", err)
// 		http.Error(w, "Internal Error", http.StatusInternalServerError)
// 		return
// 	}

// 	if user != nil {
// 		// User is already logged in.
// 		if target := r.Form.Get("redirect-target"); target != "" {
// 			http.Redirect(w, r, target, http.StatusFound)
// 			return
// 		}
// 		http.Redirect(w, r, "/", http.StatusFound)
// 		return
// 	}

// 	session, err := u.db.SessionFromPassword(ctx, r.PostForm.Get("email"), r.PostForm.Get("password"))
// 	if err == dblayer.ErrEmailMustNotBeEmpty {
// 		http.Redirect(w, r, logInLink("Email must not be empty", r.Form.Get("redirect-target")), http.StatusFound)
// 		return
// 	}
// 	if err == dblayer.ErrPasswordMustNotBeEmpty {
// 		http.Redirect(w, r, logInLink("Password must not be empty", r.Form.Get("redirect-target")), http.StatusFound)
// 		return
// 	}
// 	if err == dblayer.ErrUnknownUserOrWrongPassword {
// 		http.Redirect(w, r, logInLink("Unknown user or wrong password", r.Form.Get("redirect-target")), http.StatusFound)
// 		return
// 	}
// 	if err != nil {
// 		glog.Errorf("Error while processing log in form: %v", err)
// 		http.Error(w, "Internal Error", http.StatusInternalServerError)
// 		return
// 	}

// 	cookie := &http.Cookie{
// 		Name:     "ArtBoard-Session",
// 		Value:    session.Cookie,
// 		SameSite: http.SameSiteStrictMode,
// 		Expires:  session.Expires,
// 	}

// 	// User successfully logged in
// 	http.SetCookie(w, cookie)

// 	target := r.Form.Get("redirect-target")
// 	if target == "" {
// 		target = "/"
// 	}

// 	http.Redirect(w, r, target, http.StatusFound)
// }

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
		Name:   "ArtBoard-Session",
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
		Name:     "ArtBoard-Session",
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
