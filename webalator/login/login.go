package login

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"google.golang.org/api/idtoken"
)

type loginDetails struct {
	cookie    string
	principal string
	expiresAt time.Time
}

type Oracle struct {
	validator *idtoken.Validator

	lock   sync.Mutex
	logins map[string]*loginDetails
}

func (o *Oracle) loginWithIdentityToken(ctx context.Context, token string) (*loginDetails, error) {
	p, err := o.validator.Validate(ctx, token, "")
	if err != nil {
		return nil, fmt.Errorf("while validating token: %w", err)
	}

	o.lock.Lock()
	defer o.lock.Unlock()

	// Remove expired logins
	for c, d := range o.logins {
		if time.Now().After(d.expiresAt) {
			delete(o.logins, c)
		}
	}

	// Does this map to an existing, unexpired login?
	for _, d := range o.logins {
		if p.Claims["email"] == d.principal {
			return d, nil
		}
	}

	// Generate a strong, non-colliding cookie.
	var cookie string
	for {
		cookieBytes := make([]byte, 32)
		_, err := rand.Read(cookieBytes)
		if err != nil {
			return nil, fmt.Errorf("while making cookie: %w", err)
		}

		cookie = base64.StdEncoding.EncodeToString(cookieBytes)

		if _, ok := o.logins[cookie]; !ok {
			break
		}
	}

	o.logins[cookie] = &loginDetails{
		cookie:    cookie,
		principal: p.Claims["email"].(string),
		expiresAt: time.Now().Add(18 * time.Hour),
	}

	return o.logins[cookie], nil
}

func (o *Oracle) IsCookieValid(cookie string) bool {
	o.lock.Lock()
	defer o.lock.Unlock()

	// Remove expired logins
	for c, d := range o.logins {
		if time.Now().After(d.expiresAt) {
			delete(o.logins, c)
		}
	}

	d, ok := o.logins[cookie]
	if !ok {
		return false
	}

	return d.principal == "ahmed.taahir@gmail.com"
}

type Handler struct {
	oracle *Oracle
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("login: handling path=%q", r.URL)
	path := r.URL.EscapedPath()

	if path == "/login/token/exchange" {
		h.serveExchangeToken(w, r)
		return
	}
}

func (h *Handler) serveExchangeToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		log.Printf("login: serveExchangeToken: error while parsing form: %v", err)
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	if len(r.Form["id-token"]) != 1 {
		http.Error(w, "", http.StatusBadRequest)
		return
	}
	idToken := r.Form["id-token"][0]

	details, err := h.oracle.loginWithIdentityToken(r.Context(), idToken)
	if err != nil {
		log.Printf("login: serveExchangeToken: error while logging in: %v", err)
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:    "auth-token",
		Value:   details.cookie,
		Expires: details.expiresAt,
	})
	w.Write([]byte("Logged In"))
}
