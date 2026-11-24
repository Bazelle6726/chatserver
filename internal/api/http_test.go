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
	"golang.org/x/crypto/bcrypt"

	"chatserver/internal/ratelimit"
	"chatserver/internal/service"
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
	return &testServer{t: t, router: New(newSvc(), ratelimit.Limits{}).Router()}
}

// newSvc returns a service with the fastest bcrypt cost, so tests don't
// spend their time hashing passwords.
func newSvc() *service.Service {
	return newSvcOn(store.NewMemoryStore())
}

func newSvcOn(st *store.MemoryStore) *service.Service {
	return service.New(st).WithPasswordCost(bcrypt.MinCost)
}

// testPassword is every test user's password.
const testPassword = "correct-horse"

// do sends a request. body may be nil. user is a session token, sent as
// "Authorization: Bearer <token>"; "" means not logged in.
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
		req.Header.Set("Authorization", "Bearer "+user)
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

// mustUser signs a user up through the API and returns their session token.
func (s *testServer) mustUser(name string) string {
	s.t.Helper()
	rec := s.do("POST", "/users", map[string]string{"name": name, "password": testPassword}, "")
	expectStatus(s.t, rec, http.StatusCreated, "POST /users "+name)
	return decode[authDTO](s.t, rec).Token
}

// idOf returns the user ID of the session token's user, from GET /me.
func (s *testServer) idOf(token string) string {
	s.t.Helper()
	rec := s.do("GET", "/me", nil, token)
	expectStatus(s.t, rec, http.StatusOK, "GET /me")
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

	rec := s.do("POST", "/users", map[string]string{"name": "alice", "password": testPassword}, "")
	expectStatus(t, rec, http.StatusCreated, "create alice")
	got := decode[authDTO](t, rec)
	if got.User.Name != "alice" || got.User.Role != "member" || got.User.ID == "" || got.Token == "" {
		t.Errorf("body = %+v, want alice, member, an ID and a token", got)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookie || cookies[0].Value != got.Token ||
		!cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Errorf("cookies = %+v, want one HttpOnly, SameSite=Strict %s cookie holding the token", cookies, sessionCookie)
	}

	rec = s.do("POST", "/users", map[string]string{"name": "alice", "password": testPassword}, "")
	expectStatus(t, rec, http.StatusConflict, "duplicate name")

	rec = s.do("POST", "/users", map[string]string{"name": "x", "password": testPassword}, "")
	expectStatus(t, rec, http.StatusBadRequest, "name too short")
	if body := decode[map[string]string](t, rec); body["field"] != "name" || body["error"] == "" {
		t.Errorf("validation body = %v, want error text and field \"name\"", body)
	}

	rec = s.do("POST", "/users", map[string]string{"name": "bobby", "password": "short"}, "")
	expectStatus(t, rec, http.StatusBadRequest, "password too short")
	if body := decode[map[string]string](t, rec); body["field"] != "password" {
		t.Errorf("validation body = %v, want field \"password\"", body)
	}

	rec = s.do("POST", "/users", map[string]string{"name": "carol"}, "")
	expectStatus(t, rec, http.StatusBadRequest, "missing password")
}

func TestGetUser(t *testing.T) {
	s := newServer(t)
	bob := s.mustUser("bob")

	rec := s.do("GET", "/users/"+s.idOf(bob), nil, bob)
	expectStatus(t, rec, http.StatusOK, "get existing user")
	if got := decode[userDTO](t, rec); got.Name != "bob" {
		t.Errorf("name = %q, want bob", got.Name)
	}

	rec = s.do("GET", "/users/u-404", nil, bob)
	expectStatus(t, rec, http.StatusNotFound, "get missing user")

	rec = s.do("GET", "/users/"+s.idOf(bob), nil, "")
	expectStatus(t, rec, http.StatusUnauthorized, "looking users up needs a login")
}

func TestAuthRequired(t *testing.T) {
	s := newServer(t)

	rec := s.do("GET", "/rooms", nil, "")
	expectStatus(t, rec, http.StatusUnauthorized, "no session")

	rec = s.do("GET", "/rooms", nil, "made-up-token")
	expectStatus(t, rec, http.StatusUnauthorized, "a made-up token")

	req := httptest.NewRequest("GET", "/rooms", nil)
	req.Header.Set("Authorization", "Basic YWxpY2U6c2VjcmV0") // not a Bearer token
	rec = httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	expectStatus(t, rec, http.StatusUnauthorized, "a non-Bearer Authorization header")
}

func TestLoginAndLogout(t *testing.T) {
	s := newServer(t)
	first := s.mustUser("alice")

	rec := s.do("POST", "/sessions", map[string]string{"name": "alice", "password": "wrong-password"}, "")
	expectStatus(t, rec, http.StatusUnauthorized, "wrong password")
	rec = s.do("POST", "/sessions", map[string]string{"name": "nobody", "password": testPassword}, "")
	expectStatus(t, rec, http.StatusUnauthorized, "unknown name")

	rec = s.do("POST", "/sessions", map[string]string{"name": "Alice", "password": testPassword}, "")
	expectStatus(t, rec, http.StatusOK, "login (name in any case)")
	second := decode[authDTO](t, rec)
	if second.User.Name != "alice" || second.Token == "" || second.Token == first {
		t.Errorf("login = %+v, want alice and a new token", second)
	}

	expectStatus(t, s.do("DELETE", "/sessions", nil, first), http.StatusNoContent, "logout")
	expectStatus(t, s.do("GET", "/me", nil, first), http.StatusUnauthorized, "GET /me after logout")
	expectStatus(t, s.do("GET", "/me", nil, second.Token), http.StatusOK, "the other session still works")
}

// The browser never handles the token: the cookie from signing up logs in
// every later request, including the live feed's EventSource, which can't
// set headers. Logging out clears the cookie.
func TestSessionCookie(t *testing.T) {
	s := newServer(t)
	rec := s.do("POST", "/users", map[string]string{"name": "alice", "password": testPassword}, "")
	expectStatus(t, rec, http.StatusCreated, "sign up")
	cookie := rec.Result().Cookies()[0]

	withCookie := func(method, path string, c *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.AddCookie(c)
		rec := httptest.NewRecorder()
		s.router.ServeHTTP(rec, req)
		return rec
	}
	expectStatus(t, withCookie("GET", "/me", cookie), http.StatusOK, "GET /me with only the cookie")

	rec = withCookie("DELETE", "/sessions", cookie)
	expectStatus(t, rec, http.StatusNoContent, "logout with the cookie")
	if cleared := rec.Result().Cookies(); len(cleared) != 1 || cleared[0].MaxAge >= 0 {
		t.Errorf("logout cookies = %+v, want the session cookie deleted", cleared)
	}
	expectStatus(t, withCookie("GET", "/me", cookie), http.StatusUnauthorized, "the old cookie after logout")
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
	if m.ID == "" || m.Room != r || m.Author != s.idOf(u) || m.AuthorName != "alice" || m.Kind != "text" || m.Text != "hello gophers" || m.CreatedAt.IsZero() {
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
	expectStatus(t, s.do("GET", "/rooms/"+r+"/messages?limit=0", nil, u), http.StatusOK, "limit=0 means the default, as over gRPC")

	for _, bad := range []string{"abc", "-1", "501"} {
		rec = s.do("GET", "/rooms/"+r+"/messages?limit="+bad, nil, u)
		expectStatus(t, rec, http.StatusBadRequest, "limit="+bad)
	}

	rec = s.do("GET", "/rooms/r-404/messages", nil, u)
	expectStatus(t, rec, http.StatusNotFound, "history of missing room")
}

func TestRateLimitMiddleware(t *testing.T) {
	st := store.NewMemoryStore()
	router := New(newSvcOn(st), ratelimit.Limits{PerUser: ratelimit.New(2, 2)}).Router() // 2 a second, bursts of 2
	s := &testServer{t: t, router: router}
	u := s.mustUser("alice")

	expectStatus(t, s.do("GET", "/rooms", nil, u), http.StatusOK, "request 1")
	expectStatus(t, s.do("GET", "/rooms", nil, u), http.StatusOK, "request 2")
	expectStatus(t, s.do("GET", "/rooms", nil, u), http.StatusTooManyRequests, "request 3 in the same second")
}
