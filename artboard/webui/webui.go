package webui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"

	"row-major/artboard/dblayer"
	"row-major/artboard/dbtypes"
	"row-major/artboard/webui/uitemplates"
)

type WebUI struct {
	db *dblayer.DB
}

func New(db *dblayer.DB) *WebUI {
	return &WebUI{
		db: db,
	}
}

// Register adds the UI's handlers to m.  If host is non-empty, the handlers
// only match requests with that Host header.
func (u *WebUI) Register(m *http.ServeMux, host string) {
	m.HandleFunc("GET "+host+"/{$}", u.homeHandler)
	m.HandleFunc("GET "+host+"/log-in", u.logInGetHandler)
	m.HandleFunc("POST "+host+"/log-in", u.logInPostHandler)
	m.HandleFunc("GET "+host+"/log-out", u.logOutGetHandler)
	m.HandleFunc("POST "+host+"/log-out", u.logOutPostHandler)

	// Catch-all so that requests for this host never fall through to other
	// sites registered on the same mux.
	m.HandleFunc(host+"/", http.NotFound)
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
		slog.InfoContext(ctx, "No logged-in user because there was no session cookie.")
		return "", nil, nil
	}

	user, err := u.db.UserFromSessionCookie(ctx, sessionCookie.Value)
	if err != nil {
		return "", nil, fmt.Errorf("while getting user from session cookie: %w", err)
	}
	if user == nil {
		// Session cookie didn't correspond to an active session.
		return "", nil, nil
	}

	slog.InfoContext(ctx, "Determined logged-in user",
		slog.Group("user",
			slog.String("id", user.ID),
			slog.String("email", user.Email),
		),
	)

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
		slog.InfoContext(ctx, "No logged-in user because there was no session cookie.  Redirecting to login.")
		http.Redirect(w, r, logInLink("", redirectAfterLogin), http.StatusFound)
		return nil
	}

	user, err := u.db.UserFromSessionCookie(ctx, sessionCookie.Value)
	if err != nil {
		slog.ErrorContext(ctx, "Error while validating session cookie", slog.Any("err", err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return nil
	}
	if user == nil {
		// User is not logged in.  For example, there was a session cookie, but
		// it corresponds to an expired session.
		slog.InfoContext(ctx, "Session cookie didn't correspond to an active session.")
		http.Redirect(w, r, logInLink("", redirectAfterLogin), http.StatusFound)
		return nil
	}

	slog.InfoContext(ctx, "Determined logged-in user",
		slog.Group("user",
			slog.String("id", user.ID),
			slog.String("email", user.Email),
		),
	)

	return user
}

// homeHandler renders the home page.
func (u *WebUI) homeHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	params := &uitemplates.HomeParams{}

	_, user, err := u.getLoggedInUser(r.Context(), r)
	if err != nil {
		slog.ErrorContext(ctx, "Error while getting logged-in user", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}
	if user != nil {
		params.LoggedIn = true
	}

	content := bytes.Buffer{}
	if err := uitemplates.HomeTemplate.Execute(&content, params); err != nil {
		slog.ErrorContext(ctx, "Error while executing template", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if _, err := io.Copy(w, &content); err != nil {
		// It's too late to write an error to the HTTP response.
		slog.ErrorContext(ctx, "Error while writing output", slog.Any("err", err))
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

func (u *WebUI) logInGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, user, err := u.getLoggedInUser(ctx, r)
	if err != nil {
		slog.ErrorContext(ctx, "Error while getting logged-in user", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if err := r.ParseForm(); err != nil {
		slog.ErrorContext(ctx, "Error while parsing form", slog.Any("err", err))
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
		UserError: r.Form.Get("user-error"),
	}

	content := bytes.Buffer{}
	if err := uitemplates.LogInTemplate.Execute(&content, params); err != nil {
		slog.ErrorContext(ctx, "Error while executing template", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if _, err := io.Copy(w, &content); err != nil {
		// It's too late to write an error to the HTTP response.
		slog.ErrorContext(ctx, "Error while writing output", slog.Any("err", err))
		return
	}
}

func (u *WebUI) logInPostHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, user, err := u.getLoggedInUser(ctx, r)
	if err != nil {
		slog.ErrorContext(ctx, "Error while getting logged-in user", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if err := r.ParseForm(); err != nil {
		slog.ErrorContext(ctx, "Error while parsing form", slog.Any("err", err))
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
		slog.ErrorContext(ctx, "Error while processing log in form", slog.Any("err", err))
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

func (u *WebUI) logOutGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, user, err := u.getLoggedInUser(ctx, r)
	if err != nil {
		slog.ErrorContext(ctx, "Error while getting logged-in user", slog.Any("err", err))
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
		slog.ErrorContext(ctx, "Error while executing template", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(content); err != nil {
		// It's too late to write an error to the HTTP response.
		slog.ErrorContext(ctx, "Error while writing output", slog.Any("err", err))
		return
	}
}

func (u *WebUI) logOutPostHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	cookie, user, err := u.getLoggedInUser(ctx, r)
	if err != nil {
		slog.ErrorContext(ctx, "Error while getting logged-in user", slog.Any("err", err))
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
		slog.ErrorContext(ctx, "Error while deleting session", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:   "ArtBoard-Session",
		MaxAge: -1,
	})

	http.Redirect(w, r, "/", http.StatusFound)
}
