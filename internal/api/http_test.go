package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"

	"chatserver/internal/store"
)

// These tests send real HTTP requests to the router, without opening a port:
// httptest.NewRecorder captures the response in memory.

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)      // no debug banner
	gin.DefaultWriter = io.Discard // no request log lines in test output
	os.Exit(m.Run())
}

type testServer struct {
	t      *testing.T
	router http.Handler
}

func newServer(t *testing.T) *testServer {
	t.Helper()
	// A high limit so the rate limiter never gets in the way of these tests.
	return &testServer{t: t, router: New(store.NewMemoryStore(), 1000).Router()}
}

// do sends a request. body may be nil; user "" means no X-User-ID header.
func (s *testServer) do(method, path string, body any, user string) *httptest.ResponseRecorder {
	s.t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			s.t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if user != "" {
		req.Header.Set("X-User-ID", user)
	}
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

func expectStatus(t *testing.T, rec *httptest.ResponseRecorder, want int, what string) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("%s: status %d, want %d (body: %s)", what, rec.Code, want, rec.Body.String())
	}
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("response is not the expected JSON: %v (body: %s)", err, rec.Body.String())
	}
	return v
}

// mustUser creates a user through the API and returns its id.
func (s *testServer) mustUser(name string) string {
	s.t.Helper()
	rec := s.do("POST", "/users", map[string]string{"name": name}, "")
	expectStatus(s.t, rec, http.StatusCreated, "POST /users "+name)
	return decode[userDTO](s.t, rec).ID
}

func (s *testServer) mustRoom(user, name string) string {
	s.t.Helper()
	rec := s.do("POST", "/rooms", map[string]string{"name": name}, user)
	expectStatus(s.t, rec, http.StatusCreated, "POST /rooms "+name)
	return decode[roomDTO](s.t, rec).ID
}

func TestCreateUser(t *testing.T) {
	s := newServer(t)

	rec := s.do("POST", "/users", map[string]string{"name": "alice"}, "")
	expectStatus(t, rec, http.StatusCreated, "create alice")
	if got := decode[userDTO](t, rec); got != (userDTO{ID: "u-1", Name: "alice", Role: "member"}) {
		t.Errorf("body = %+v, want {u-1 alice member}", got)
	}

	rec = s.do("POST", "/users", map[string]string{"name": "alice"}, "")
	expectStatus(t, rec, http.StatusConflict, "duplicate name")

	rec = s.do("POST", "/users", map[string]string{"name": "x"}, "")
	expectStatus(t, rec, http.StatusBadRequest, "name too short")
	if body := decode[map[string]string](t, rec); body["field"] != "name" || body["error"] == "" {
		t.Errorf("validation body = %v, want error text and field \"name\"", body)
	}

	rec = s.do("POST", "/users", map[string]string{}, "")
	expectStatus(t, rec, http.StatusBadRequest, "missing name")
}

func TestGetUser(t *testing.T) {
	s := newServer(t)
	id := s.mustUser("bob")

	rec := s.do("GET", "/users/"+id, nil, "")
	expectStatus(t, rec, http.StatusOK, "get existing user")
	if got := decode[userDTO](t, rec); got.Name != "bob" {
		t.Errorf("name = %q, want bob", got.Name)
	}

	rec = s.do("GET", "/users/u-404", nil, "")
	expectStatus(t, rec, http.StatusNotFound, "get missing user")
}

func TestAuthRequired(t *testing.T) {
	s := newServer(t)

	rec := s.do("GET", "/rooms", nil, "")
	expectStatus(t, rec, http.StatusUnauthorized, "no X-User-ID header")

	rec = s.do("GET", "/rooms", nil, "u-404")
	expectStatus(t, rec, http.StatusUnauthorized, "unknown user")
}

func TestCreateAndListRooms(t *testing.T) {
	s := newServer(t)
	u := s.mustUser("alice")

	s.mustRoom(u, "random")
	s.mustRoom(u, "general")

	rec := s.do("POST", "/rooms", map[string]string{"name": "general"}, u)
	expectStatus(t, rec, http.StatusConflict, "duplicate room")

	rec = s.do("POST", "/rooms", map[string]any{"name": "big", "max_history": 5000}, u)
	expectStatus(t, rec, http.StatusBadRequest, "max_history too large")

	rec = s.do("GET", "/rooms", nil, u)
	expectStatus(t, rec, http.StatusOK, "list rooms")
	rooms := decode[[]roomDTO](t, rec)
	if len(rooms) != 2 || rooms[0].Name != "general" || rooms[1].Name != "random" {
		t.Errorf("rooms = %+v, want general then random", rooms)
	}
}

func TestListRoomsEmpty(t *testing.T) {
	s := newServer(t)
	u := s.mustUser("alice")
	rec := s.do("GET", "/rooms", nil, u)
	expectStatus(t, rec, http.StatusOK, "list with no rooms")
	if got := rec.Body.String(); got != "[]" {
		t.Errorf("body = %s, want [] (an empty list, not null)", got)
	}
}

func TestJoinAndPost(t *testing.T) {
	s := newServer(t)
	u := s.mustUser("alice")
	r := s.mustRoom(u, "general")

	rec := s.do("POST", "/rooms/"+r+"/messages", map[string]string{"text": "hi"}, u)
	expectStatus(t, rec, http.StatusForbidden, "post before joining")

	rec = s.do("POST", "/rooms/"+r+"/members", nil, u)
	expectStatus(t, rec, http.StatusNoContent, "join")

	rec = s.do("POST", "/rooms/"+r+"/members", nil, u)
	expectStatus(t, rec, http.StatusConflict, "join twice")

	rec = s.do("POST", "/rooms/r-404/members", nil, u)
	expectStatus(t, rec, http.StatusNotFound, "join missing room")

	rec = s.do("POST", "/rooms/"+r+"/messages", map[string]string{"text": "hello gophers"}, u)
	expectStatus(t, rec, http.StatusCreated, "post")
	m := decode[messageDTO](t, rec)
	if m.ID == "" || m.Room != r || m.Author != u || m.Kind != "text" || m.Text != "hello gophers" || m.CreatedAt.IsZero() {
		t.Errorf("message = %+v", m)
	}

	rec = s.do("POST", "/rooms/"+r+"/messages", map[string]string{"text": "   "}, u)
	expectStatus(t, rec, http.StatusBadRequest, "blank text")
}

func TestHistory(t *testing.T) {
	s := newServer(t)
	u := s.mustUser("alice")
	r := s.mustRoom(u, "general")
	expectStatus(t, s.do("POST", "/rooms/"+r+"/members", nil, u), http.StatusNoContent, "join")
	for _, text := range []string{"one", "two", "three"} {
		expectStatus(t, s.do("POST", "/rooms/"+r+"/messages", map[string]string{"text": text}, u), http.StatusCreated, "post "+text)
	}

	rec := s.do("GET", "/rooms/"+r+"/messages?limit=2", nil, u)
	expectStatus(t, rec, http.StatusOK, "history limit=2")
	got := decode[[]messageDTO](t, rec)
	if len(got) != 2 || got[0].Text != "two" || got[1].Text != "three" {
		t.Errorf("history = %+v, want two, three (newest 2, oldest first)", got)
	}

	rec = s.do("GET", "/rooms/"+r+"/messages", nil, u)
	expectStatus(t, rec, http.StatusOK, "history default limit")
	if got := decode[[]messageDTO](t, rec); len(got) != 3 {
		t.Errorf("default limit returned %d messages, want 3", len(got))
	}

	for _, bad := range []string{"abc", "0", "501"} {
		rec = s.do("GET", "/rooms/"+r+"/messages?limit="+bad, nil, u)
		expectStatus(t, rec, http.StatusBadRequest, "limit="+bad)
	}

	rec = s.do("GET", "/rooms/r-404/messages", nil, u)
	expectStatus(t, rec, http.StatusNotFound, "history of missing room")
}

func TestRateLimitMiddleware(t *testing.T) {
	st := store.NewMemoryStore()
	router := New(st, 2).Router() // only 2 requests per second
	s := &testServer{t: t, router: router}
	u := s.mustUser("alice")

	expectStatus(t, s.do("GET", "/rooms", nil, u), http.StatusOK, "request 1")
	expectStatus(t, s.do("GET", "/rooms", nil, u), http.StatusOK, "request 2")
	expectStatus(t, s.do("GET", "/rooms", nil, u), http.StatusTooManyRequests, "request 3 in the same second")
}
