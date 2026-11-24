package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chatserver/internal/ratelimit"
)

const testGameKey = "a-long-enough-game-key"

// gameServer is a test server with the game API on.
func gameServer(t *testing.T) *testServer {
	t.Helper()
	return &testServer{t: t, router: New(newSvc(), ratelimit.Limits{}).WithGame(testGameKey).Router()}
}

// doGame sends a game API request with key ("" = no X-Game-Key header).
func (s *testServer) doGame(path, body, key string) *httptest.ResponseRecorder {
	s.t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set(gameKeyHeader, key)
	}
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

func TestGameAPI(t *testing.T) {
	s := gameServer(t)

	// Without the right key, nothing.
	expectStatus(t, s.doGame("/game/sessions", `{"player_id":"steam:1","name":"Ace"}`, ""), http.StatusUnauthorized, "no game key")
	expectStatus(t, s.doGame("/game/sessions", `{"player_id":"steam:1","name":"Ace"}`, "wrong-key"), http.StatusUnauthorized, "wrong game key")

	// The game signs two players in; each gets a token that works like a login.
	rec := s.doGame("/game/sessions", `{"player_id":"steam:1","name":"Ace"}`, testGameKey)
	expectStatus(t, rec, http.StatusOK, "sign in Ace")
	ace := decode[authDTO](t, rec)
	if ace.User.Name != "Ace" || ace.Token == "" || len(rec.Result().Cookies()) != 0 {
		t.Errorf("game session = %+v (cookies %v), want Ace, a token and no cookie", ace, rec.Result().Cookies())
	}
	expectStatus(t, s.do("GET", "/me", nil, ace.Token), http.StatusOK, "the game-issued token logs in")
	rec = s.doGame("/game/sessions", `{"player_id":"steam:2","name":"Bo Bo"}`, testGameKey)
	expectStatus(t, rec, http.StatusOK, "sign in Bo Bo")
	bo := decode[authDTO](t, rec)
	if bo.User.Name != "Bo_Bo" {
		t.Errorf("name = %q, want Bo_Bo (spaces become underscores)", bo.User.Name)
	}

	// The game makes a private match room with Ace in it.
	rec = s.doGame("/game/rooms", `{"name":"match-1","players":["steam:1"]}`, testGameKey)
	expectStatus(t, rec, http.StatusOK, "create the match room")
	room := decode[roomDTO](t, rec)
	if !room.Private || room.Members != 1 {
		t.Errorf("match room = %+v, want private with 1 member", room)
	}

	// Ace can read it; Bo can't join it or see it.
	expectStatus(t, s.do("GET", "/rooms/"+room.ID+"/messages", nil, ace.Token), http.StatusOK, "Ace reads the match room")
	expectStatus(t, s.do("POST", "/rooms/"+room.ID+"/members", nil, bo.Token), http.StatusForbidden, "Bo joins a private room")
	if rooms := decode[[]roomDTO](t, s.do("GET", "/rooms", nil, bo.Token)); len(rooms) != 0 {
		t.Errorf("Bo sees %+v, want no rooms", rooms)
	}

	// Unknown players and missing fields are refused.
	expectStatus(t, s.doGame("/game/rooms", `{"name":"match-2","players":["steam:404"]}`, testGameKey), http.StatusNotFound, "a player who never signed in")
	expectStatus(t, s.doGame("/game/sessions", `{"name":"Ace"}`, testGameKey), http.StatusBadRequest, "no player_id")
}

// With no key configured, the game API doesn't exist.
func TestGameAPIOff(t *testing.T) {
	s := newServer(t)
	expectStatus(t, s.doGame("/game/sessions", `{"player_id":"steam:1"}`, "anything"), http.StatusNotFound, "game API off")
}
