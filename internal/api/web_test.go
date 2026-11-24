package api

import (
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	"chatserver/internal/ratelimit"
)

// fakeApp stands in for the built React app: fstest.MapFS is an in-memory
// file system, so the test doesn't depend on npm run build.
var fakeApp = fstest.MapFS{
	"index.html":       {Data: []byte(`<!doctype html><title>chatserver</title><div id="root"></div>`)},
	"assets/index.js":  {Data: []byte(`console.log("hello")`)},
	"assets/index.css": {Data: []byte(`body { margin: 0 }`)},
}

func newWebServer(t *testing.T) *testServer {
	t.Helper()
	return &testServer{t: t, router: New(newSvc(), ratelimit.Limits{}).WithWeb(fakeApp).Router()}
}

// The app's files are served for paths no API route matches.
func TestServesTheWebApp(t *testing.T) {
	s := newWebServer(t)

	rec := s.do("GET", "/", nil, "")
	expectStatus(t, rec, http.StatusOK, "GET /")
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("GET / Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(rec.Body.String(), `<div id="root">`) {
		t.Errorf("GET / didn't return index.html: %q", rec.Body.String())
	}

	rec = s.do("GET", "/assets/index.js", nil, "")
	expectStatus(t, rec, http.StatusOK, "GET /assets/index.js")
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("JS Content-Type = %q, want text/javascript", ct)
	}
	expectStatus(t, s.do("GET", "/assets/index.css", nil, ""), http.StatusOK, "GET /assets/index.css")
	expectStatus(t, s.do("GET", "/missing.txt", nil, ""), http.StatusNotFound, "GET a file that isn't there")
	expectStatus(t, s.do("POST", "/nowhere", map[string]string{}, ""), http.StatusNotFound, "POST to a path with no route")

	// The API is unaffected: its routes still win.
	expectStatus(t, s.do("POST", "/users", map[string]string{"name": "alice", "password": testPassword}, ""), http.StatusCreated, "POST /users")
	expectStatus(t, s.do("GET", "/rooms", nil, ""), http.StatusUnauthorized, "GET /rooms without a user still goes to the API")
}

// No web app configured, no files served.
func TestNoWebAppWithoutWithWeb(t *testing.T) {
	expectStatus(t, newServer(t).do("GET", "/", nil, ""), http.StatusNotFound, "GET / on an API without WithWeb")
}

// The old ?user= login is gone: a user ID is not a credential.
func TestNoUserQueryLogin(t *testing.T) {
	s := newServer(t)
	id := s.idOf(s.mustUser("alice"))
	expectStatus(t, s.do("GET", "/rooms?user="+id, nil, ""), http.StatusUnauthorized, "?user= with a real user ID")
}
