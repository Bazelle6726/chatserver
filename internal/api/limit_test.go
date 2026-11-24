package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"chatserver/internal/ratelimit"
	"chatserver/internal/service"
	"chatserver/internal/store"
)

// A body over MaxBodyBytes must be refused with 413, whether the client says
// how big it is up front (Content-Length) or not (chunked).
func TestBodyTooLarge(t *testing.T) {
	s := newServer(t)
	huge := `{"name":"` + strings.Repeat("a", MaxBodyBytes) + `"}`

	for _, tc := range []struct {
		name    string
		chunked bool
	}{
		{"with Content-Length", false},
		{"chunked", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(huge))
			req.Header.Set("Content-Type", "application/json")
			if tc.chunked {
				req.ContentLength = -1 // unknown length, as with chunked encoding
			}
			rec := httptest.NewRecorder()
			s.router.ServeHTTP(rec, req)
			expectStatus(t, rec, http.StatusRequestEntityTooLarge, "huge body")
		})
	}

	// A normal body still works.
	expectStatus(t, s.do(http.MethodPost, "/users", map[string]string{"name": "alice"}, ""), http.StatusCreated, "normal body")
}

// Sign-ups are limited per IP; other public routes only by the looser
// per-IP limit.
func TestSignUpLimit(t *testing.T) {
	limits := ratelimit.Limits{SignUps: ratelimit.New(ratelimit.Per(2, time.Minute), 2)}
	s := &testServer{t: t, router: New(service.New(store.NewMemoryStore()), limits).Router()}

	id := s.mustUser("alice")
	s.mustUser("bobby")
	expectStatus(t, s.do("POST", "/users", map[string]string{"name": "carol"}, ""), http.StatusTooManyRequests, "3rd sign-up in a minute")
	expectStatus(t, s.do("GET", "/users/"+id, nil, ""), http.StatusOK, "looking a user up isn't a sign-up")
}

// Requests without a login are limited per IP.
func TestPerIPLimit(t *testing.T) {
	limits := ratelimit.Limits{PerIP: ratelimit.New(1, 3)}
	s := &testServer{t: t, router: New(service.New(store.NewMemoryStore()), limits).Router()}
	for i := range 3 {
		expectStatus(t, s.do("GET", "/users/u-404", nil, ""), http.StatusNotFound, fmt.Sprint("lookup ", i+1))
	}
	expectStatus(t, s.do("GET", "/users/u-404", nil, ""), http.StatusTooManyRequests, "4th lookup in a burst of 3")
}
