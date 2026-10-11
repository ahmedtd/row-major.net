package sitecontent

import (
	"bufio"
	"bytes"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type route struct {
	kind        string
	servingPath string
	arg         string
}

func loadRoutes(t *testing.T) []route {
	t.Helper()

	f, err := os.Open("testdata/routes.txt")
	if err != nil {
		t.Fatalf("Error opening routes: %v", err)
	}
	defer f.Close()

	var routes []route
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			t.Fatalf("Malformed route line %q", line)
		}
		routes = append(routes, route{kind: fields[0], servingPath: fields[1], arg: fields[2]})
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("Error reading routes: %v", err)
	}

	return routes
}

func serve(h http.Handler, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestServesExactlyTheExpectedRoutes(t *testing.T) {
	h, err := NewEmbeddedHandler()
	if err != nil {
		t.Fatalf("Error creating handler: %v", err)
	}

	var want []string
	for _, r := range loadRoutes(t) {
		want = append(want, r.servingPath)
	}
	sort.Strings(want)

	var got []string
	for p := range h.routes {
		got = append(got, p)
	}
	sort.Strings(got)

	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("Wrong set of routes (-got +want):\n%s", diff)
	}
}

func TestRoutes(t *testing.T) {
	h, err := NewEmbeddedHandler()
	if err != nil {
		t.Fatalf("Error creating handler: %v", err)
	}

	for _, r := range loadRoutes(t) {
		t.Run(r.servingPath, func(t *testing.T) {
			rec := serve(h, http.MethodGet, r.servingPath)

			switch r.kind {
			case "static":
				if rec.Code != http.StatusOK {
					t.Fatalf("Wrong status; got %d, want %d", rec.Code, http.StatusOK)
				}
				if got := rec.Header().Get("Content-Type"); got != r.arg {
					t.Errorf("Wrong Content-Type; got %q, want %q", got, r.arg)
				}
				wantBody, err := fs.ReadFile(Static(), strings.TrimPrefix(r.servingPath, "/"))
				if err != nil {
					t.Fatalf("Error reading static file: %v", err)
				}
				if !bytes.Equal(rec.Body.Bytes(), wantBody) {
					t.Errorf("Body doesn't match static file")
				}

			case "template":
				if rec.Code != http.StatusOK {
					t.Fatalf("Wrong status; got %d, want %d", rec.Code, http.StatusOK)
				}
				if got, want := rec.Header().Get("Content-Type"), "text/html; charset=utf-8"; got != want {
					t.Errorf("Wrong Content-Type; got %q, want %q", got, want)
				}
				// The base template provides the document structure, and every
				// page specializes the title.
				body := rec.Body.String()
				if !strings.HasPrefix(body, "<!DOCTYPE html>") {
					t.Errorf("Body doesn't start with the base template")
				}
				if strings.Contains(body, "<title>Title - ") {
					t.Errorf("Body has the default title; specialization %v wasn't applied", r.arg)
				}

			case "redirect":
				if rec.Code != http.StatusFound {
					t.Fatalf("Wrong status; got %d, want %d", rec.Code, http.StatusFound)
				}
				if got := rec.Header().Get("Location"); got != r.arg {
					t.Errorf("Wrong Location; got %q, want %q", got, r.arg)
				}

			default:
				t.Fatalf("Unknown route kind %q", r.kind)
			}
		})
	}
}

func TestNotFound(t *testing.T) {
	h, err := NewEmbeddedHandler()
	if err != nil {
		t.Fatalf("Error creating handler: %v", err)
	}

	for _, p := range []string{
		"/does-not-exist",
		"/masters-thesis-presentation/",
		"/articles/",
		"/base.html.tmpl",
		"/index.html.tmpl",
	} {
		if rec := serve(h, http.MethodGet, p); rec.Code != http.StatusNotFound {
			t.Errorf("GET %v: wrong status; got %d, want %d", p, rec.Code, http.StatusNotFound)
		}
	}
}

func TestStaticRange(t *testing.T) {
	h, err := NewEmbeddedHandler()
	if err != nil {
		t.Fatalf("Error creating handler: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/row-major-logo-optimized.svg", nil)
	req.Header.Set("Range", "bytes=0-4")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("Wrong status; got %d, want %d", rec.Code, http.StatusPartialContent)
	}
	if got, want := rec.Body.String(), "<?xml"; got != want {
		t.Errorf("Wrong body; got %q, want %q", got, want)
	}
}
