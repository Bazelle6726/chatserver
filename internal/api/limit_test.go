package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
