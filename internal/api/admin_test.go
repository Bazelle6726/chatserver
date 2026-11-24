package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"chatserver/internal/moderation"
	"chatserver/internal/service"
	"chatserver/internal/store"
)

// adminServer runs the API with a banned-word list saved to a temp file.
type adminServer struct {
	t      *testing.T
	router http.Handler
	words  *moderation.List
	path   string
}

func newAdminServer(t *testing.T, token string) *adminServer {
	t.Helper()
	path := filepath.Join(t.TempDir(), "banned_words.txt")
	words := moderation.New("scam", "spam")
	a := New(service.New(store.NewMemoryStore()), 1000).WithModeration(words, path, token)
	return &adminServer{t: t, router: a.Router(), words: words, path: path}
}

// do sends an admin request. token "" sends no X-Admin-Token header.
func (s *adminServer) do(method, path string, body any, token string) *httptest.ResponseRecorder {
	s.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("X-Admin-Token", token)
	}
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

// AdminOnly
func TestAdminOnly(t *testing.T) {
	off := newAdminServer(t, "")
	expectStatus(t, off.do("GET", "/admin/banned-words", nil, "anything"), http.StatusNotFound, "admin API with no token configured")

	s := newAdminServer(t, "s3cret")
	expectStatus(t, s.do("GET", "/admin/banned-words", nil, ""), http.StatusUnauthorized, "no X-Admin-Token")
	expectStatus(t, s.do("GET", "/admin/banned-words", nil, "wrong"), http.StatusUnauthorized, "wrong token")
	expectStatus(t, s.do("GET", "/admin/banned-words", nil, "s3cret"), http.StatusOK, "right token")
	expectStatus(t, s.do("POST", "/admin/banned-words", map[string]string{"word": "fraud"}, "wrong"), http.StatusUnauthorized, "POST with a wrong token")
	if s.words.IsBanned("fraud") {
		t.Error("a request with a wrong token changed the list")
	}
}

// Listing the words.
func TestListBannedWords(t *testing.T) {
	s := newAdminServer(t, "s3cret")
	rec := s.do("GET", "/admin/banned-words", nil, "s3cret")
	expectStatus(t, rec, http.StatusOK, "list")
	got := decode[struct {
		Words []string `json:"words"`
	}](t, rec)
	if !slices.Equal(got.Words, []string{"scam", "spam"}) {
		t.Errorf("words = %v, want [scam spam] (sorted)", got.Words)
	}
}

// Adding a word.
func TestAddBannedWord(t *testing.T) {
	s := newAdminServer(t, "s3cret")

	rec := s.do("POST", "/admin/banned-words", map[string]string{"word": "  Fraud "}, "s3cret")
	expectStatus(t, rec, http.StatusCreated, "add Fraud")
	if got := decode[map[string]string](t, rec)["word"]; got != "fraud" {
		t.Errorf("reply word = %q, want %q (trimmed, lowercase)", got, "fraud")
	}
	if !s.words.IsBanned("such FRAUD!") {
		t.Error("the word was not added to the live list")
	}
	saved, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatalf("the list was not saved to its file: %v", err)
	}
	if !strings.Contains(string(saved), "fraud") {
		t.Errorf("saved file = %q, want it to contain fraud", saved)
	}

	for _, bad := range []struct {
		name string
		body any
	}{
		{"two words", map[string]string{"word": "free money"}},
		{"punctuation", map[string]string{"word": "f*ck"}},
		{"too long", map[string]string{"word": strings.Repeat("a", 51)}},
		{"blank", map[string]string{"word": "   "}},
		{"no word field", map[string]string{}},
	} {
		rec := s.do("POST", "/admin/banned-words", bad.body, "s3cret")
		expectStatus(t, rec, http.StatusBadRequest, bad.name)
	}
}

// Removing a word.
func TestRemoveBannedWord(t *testing.T) {
	s := newAdminServer(t, "s3cret")
	expectStatus(t, s.do("DELETE", "/admin/banned-words/SCAM", nil, "s3cret"), http.StatusNoContent, "remove SCAM")
	if s.words.IsBanned("a scam") {
		t.Error("scam is still banned")
	}
	saved, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatalf("the list was not saved to its file: %v", err)
	}
	if strings.Contains(string(saved), "scam") {
		t.Errorf("saved file still contains scam: %q", saved)
	}
	expectStatus(t, s.do("DELETE", "/admin/banned-words/notthere", nil, "s3cret"), http.StatusNoContent, "remove a word that isn't banned")
}

// The routes exist only when moderation is configured.
func TestNoAdminRoutesWithoutModeration(t *testing.T) {
	s := newServer(t) // plain API, no WithModeration
	expectStatus(t, s.do("GET", "/admin/banned-words", nil, ""), http.StatusNotFound, "admin route on an API without moderation")
}

// A client that keeps guessing the admin token is locked out, even if it
// then guesses right, and a forged X-Forwarded-For doesn't get it a new IP.
func TestAdminLockout(t *testing.T) {
	s := newAdminServer(t, "s3cret")
	for i := range adminMaxFailures {
		expectStatus(t, s.do("GET", "/admin/banned-words", nil, "wrong"), http.StatusUnauthorized, fmt.Sprintf("wrong guess %d", i+1))
	}
	expectStatus(t, s.do("GET", "/admin/banned-words", nil, "s3cret"), http.StatusTooManyRequests, "right token after too many wrong ones")

	req := httptest.NewRequest("GET", "/admin/banned-words", nil)
	req.Header.Set("X-Admin-Token", "wrong")
	req.Header.Set("X-Forwarded-For", "203.0.113.7") // pretend to be someone else
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	expectStatus(t, rec, http.StatusTooManyRequests, "same client with a forged X-Forwarded-For")
}

// A 500 must not send the internal error's details to the client.
func TestInternalErrorsAreHidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/boom", func(c *gin.Context) { writeError(c, errors.New(`open C:\secret\path: access denied`)) })
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/boom", nil))
	expectStatus(t, rec, http.StatusInternalServerError, "internal error")
	if strings.Contains(rec.Body.String(), "secret") {
		t.Errorf("500 body leaks the internal error: %s", rec.Body.String())
	}
}
