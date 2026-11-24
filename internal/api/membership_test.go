package api

import (
	"net/http"
	"testing"

	"chatserver/internal/hub"
	"chatserver/internal/store"
)

// Reading a room, its history or its live feed, needs membership, the same
// as posting to it.
func TestReadingNeedsMembership(t *testing.T) {
	h := hub.NewHub(4) // never run: the stream must be refused before it's used
	s := &testServer{t: t, router: New(store.NewMemoryStore(), 1000).WithHub(h, nil).Router()}
	alice := s.mustUser("alice")
	eve := s.mustUser("eve")
	room := s.mustRoom(alice, "general")
	expectStatus(t, s.do("POST", "/rooms/"+room+"/members", nil, alice), http.StatusNoContent, "alice joins")

	expectStatus(t, s.do("GET", "/rooms/"+room+"/messages", nil, eve), http.StatusForbidden, "eve reads history without joining")
	expectStatus(t, s.do("GET", "/rooms/"+room+"/stream", nil, eve), http.StatusForbidden, "eve watches without joining")
	expectStatus(t, s.do("GET", "/rooms/"+room+"/messages", nil, alice), http.StatusOK, "alice reads history")

	expectStatus(t, s.do("POST", "/rooms/"+room+"/members", nil, eve), http.StatusNoContent, "eve joins")
	expectStatus(t, s.do("GET", "/rooms/"+room+"/messages", nil, eve), http.StatusOK, "eve reads history after joining")
}
