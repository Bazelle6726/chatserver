package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"chatserver/internal/chat"
	"chatserver/internal/hub"
	"chatserver/internal/moderation"
	"chatserver/internal/ratelimit"
	"chatserver/internal/store"
)

// testPassword is every test user's password.
const testPassword = "correct-horse"

// newTest returns a service with the fastest bcrypt cost, so tests don't
// spend their time hashing.
func newTest() *Service {
	return New(store.NewMemoryStore()).WithPasswordCost(bcrypt.MinCost)
}

// setup returns a service with users alice and eve, and a room "general"
// that only alice has joined.
func setup(t *testing.T, svc *Service) (alice, eve chat.UserID, room chat.RoomID) {
	t.Helper()
	ctx := t.Context()
	a, _, err := svc.Register(ctx, "alice", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	e, _, err := svc.Register(ctx, "eve", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	r, err := svc.CreateRoom(ctx, "general", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.JoinRoom(ctx, a.ID, r.ID); err != nil {
		t.Fatal(err)
	}
	return a.ID, e.ID, r.ID
}

func wantValidation(t *testing.T, err error, field, what string) {
	t.Helper()
	var ve *chat.ValidationError
	if !errors.As(err, &ve) || ve.Field != field {
		t.Errorf("%s: err = %v, want a ValidationError for %q", what, err, field)
	}
}

func TestCreateRoomMaxHistory(t *testing.T) {
	svc := newTest()
	ctx := t.Context()
	for _, bad := range []int{-1, MaxMaxHistory + 1} {
		_, err := svc.CreateRoom(ctx, "room", bad)
		wantValidation(t, err, "max_history", "max_history out of range")
	}
	if _, err := svc.CreateRoom(ctx, "room", 0); err != nil {
		t.Errorf("max_history 0 (the default) = %v, want nil", err)
	}
}

func TestHistory(t *testing.T) {
	svc := newTest().WithModeration(moderation.New("scam").IsBanned)
	alice, eve, room := setup(t, svc)
	ctx := t.Context()
	for i := range DefaultLimit + 5 {
		text := "hello"
		if i == DefaultLimit+4 {
			text = "a scam"
		}
		if _, err := svc.PostMessage(ctx, alice, room, text); err != nil {
			t.Fatal(err)
		}
	}

	ms, err := svc.History(ctx, alice, room, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != DefaultLimit {
		t.Errorf("limit 0 returned %d messages, want the default %d", len(ms), DefaultLimit)
	}
	if last := ms[len(ms)-1].Text; last != "[removed]" {
		t.Errorf("newest message = %q, want [removed]", last)
	}

	for _, bad := range []int{-1, MaxLimit + 1} {
		_, err := svc.History(ctx, alice, room, bad)
		wantValidation(t, err, "limit", "limit out of range")
	}
	if _, err := svc.History(ctx, eve, room, 10); !errors.Is(err, chat.ErrNotMember) {
		t.Errorf("History by a non-member = %v, want ErrNotMember", err)
	}
}

func TestPostMessagePublishes(t *testing.T) {
	var published []chat.Message
	svc := newTest().WithHub(nil, func(m chat.Message) { published = append(published, m) })
	alice, eve, room := setup(t, svc)

	m, err := svc.PostMessage(t.Context(), alice, room, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if len(published) != 1 || published[0].ID != m.ID {
		t.Errorf("published %v, want exactly the posted message", published)
	}
	if _, err := svc.PostMessage(t.Context(), eve, room, "hi"); !errors.Is(err, chat.ErrNotMember) {
		t.Errorf("post by a non-member = %v, want ErrNotMember", err)
	}
	if len(published) != 1 {
		t.Error("a refused post was published")
	}
}

func TestWatch(t *testing.T) {
	if _, _, err := newTest().Watch(t.Context(), "u-1", "r-1"); !errors.Is(err, ErrLiveOff) {
		t.Errorf("Watch without a hub = %v, want ErrLiveOff", err)
	}

	h := hub.NewHub(4)
	go h.Run(t.Context())
	svc := newTest().WithHub(h, h.Publish)
	alice, eve, room := setup(t, svc)
	ctx := t.Context()

	if _, _, err := svc.Watch(ctx, eve, room); !errors.Is(err, chat.ErrNotMember) {
		t.Errorf("Watch by a non-member = %v, want ErrNotMember", err)
	}

	// JoinAndWatch joins eve first, then watches.
	client, leave, err := svc.JoinAndWatch(ctx, eve, room)
	if err != nil {
		t.Fatal(err)
	}
	defer leave()
	if _, err := svc.PostMessage(ctx, alice, room, "hi eve"); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-client.Out():
		if m.Text != "hi eve" {
			t.Errorf("eve got %q, want %q", m.Text, "hi eve")
		}
	case <-time.After(time.Second):
		t.Fatal("eve's watch got nothing")
	}

	// JoinAndWatch is fine for someone who's already a member.
	_, leave2, err := svc.JoinAndWatch(ctx, alice, room)
	if err != nil {
		t.Errorf("JoinAndWatch by a member = %v, want nil", err)
	} else {
		leave2()
	}
}

// The post limit applies per user, however the message is sent.
func TestPostLimit(t *testing.T) {
	svc := newTest().WithPostLimit(ratelimit.New(1, 3)) // bursts of 3
	alice, eve, room := setup(t, svc)
	ctx := t.Context()
	if err := svc.JoinRoom(ctx, eve, room); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		if _, err := svc.PostMessage(ctx, alice, room, "hi"); err != nil {
			t.Fatalf("post %d: %v", i+1, err)
		}
	}
	if _, err := svc.PostMessage(ctx, alice, room, "hi"); !errors.Is(err, ratelimit.ErrLimited) {
		t.Errorf("4th post in a burst of 3 = %v, want ErrLimited", err)
	}
	if _, err := svc.PostMessage(ctx, eve, room, "hi"); err != nil {
		t.Errorf("eve has a separate limit, but got %v", err)
	}
	ms, _ := svc.History(ctx, alice, room, 0)
	if len(ms) != 4 {
		t.Errorf("history has %d messages, want 4: a refused post must not be stored", len(ms))
	}
}

func TestRegisterLoginLogout(t *testing.T) {
	svc := newTest()
	ctx := t.Context()

	u, token, err := svc.Register(ctx, "alice", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := svc.Authenticate(ctx, token); err != nil || got.ID != u.ID {
		t.Fatalf("Authenticate(sign-up token) = %v, %v; want alice", got, err)
	}

	_, _, err = svc.Register(ctx, "bob", "short")
	wantValidation(t, err, "password", "a short password")
	_, _, err = svc.Register(ctx, "x", testPassword)
	wantValidation(t, err, "name", "a short name")

	// Logging in works by name, ignoring case, and gives a new token.
	_, token2, err := svc.Login(ctx, "ALICE", testPassword)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if token2 == token {
		t.Error("a new login reused the old token")
	}

	// Wrong password and unknown name fail the same way.
	if _, _, err := svc.Login(ctx, "alice", "wrong-password"); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("wrong password = %v, want ErrBadCredentials", err)
	}
	if _, _, err := svc.Login(ctx, "nobody", testPassword); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("unknown name = %v, want ErrBadCredentials", err)
	}

	// Logging out ends that session only.
	svc.Logout(token)
	if _, err := svc.Authenticate(ctx, token); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("Authenticate after Logout = %v, want ErrUnauthenticated", err)
	}
	if _, err := svc.Authenticate(ctx, token2); err != nil {
		t.Errorf("the other session ended too: %v", err)
	}
	if _, err := svc.Authenticate(ctx, ""); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("Authenticate(\"\") = %v, want ErrUnauthenticated", err)
	}
}

// A match room is private: only players the game adds can see, join, read
// or watch it.
func TestMatchRoom(t *testing.T) {
	svc := newTest()
	alice, eve, _ := setup(t, svc)
	ctx := t.Context()

	room, err := svc.MatchRoom(ctx, "match-1", 0, []chat.UserID{alice})
	if err != nil {
		t.Fatal(err)
	}
	if !room.Private || !room.IsMember(alice) || room.IsMember(eve) {
		t.Fatalf("match room: private=%v alice=%v eve=%v; want private with only alice", room.Private, room.IsMember(alice), room.IsMember(eve))
	}

	// Eve can't get in by ID or by name, and can't see it in the list.
	if err := svc.JoinRoom(ctx, eve, room.ID); !errors.Is(err, ErrPrivateRoom) {
		t.Errorf("JoinRoom by an outsider = %v, want ErrPrivateRoom", err)
	}
	if _, err := svc.OpenRoom(ctx, eve, "match-1"); !errors.Is(err, ErrPrivateRoom) {
		t.Errorf("OpenRoom by an outsider = %v, want ErrPrivateRoom", err)
	}
	for _, who := range []struct {
		id   chat.UserID
		sees bool
	}{{alice, true}, {eve, false}} {
		rooms, _ := svc.ListRooms(ctx, who.id)
		found := false
		for _, r := range rooms {
			found = found || r.ID == room.ID
		}
		if found != who.sees {
			t.Errorf("%s sees the match room in the list = %v, want %v", who.id, found, who.sees)
		}
	}

	// Adding eve later works, and calling MatchRoom again is harmless.
	if _, err := svc.MatchRoom(ctx, "match-1", 0, []chat.UserID{alice, eve}); err != nil {
		t.Fatal(err)
	}
	if !room.IsMember(eve) {
		t.Error("eve wasn't added")
	}
	if _, err := svc.OpenRoom(ctx, eve, "match-1"); err != nil {
		t.Errorf("OpenRoom by a player the game added = %v, want nil", err)
	}

	// A public room can't be turned into a match room, and unknown players are refused.
	if _, err := svc.MatchRoom(ctx, "general", 0, nil); !errors.Is(err, chat.ErrAlreadyExists) {
		t.Errorf("MatchRoom over a public room = %v, want ErrAlreadyExists", err)
	}
	if _, err := svc.MatchRoom(ctx, "match-2", 0, []chat.UserID{"u-404"}); !errors.Is(err, chat.ErrUserNotFound) {
		t.Errorf("MatchRoom with an unknown player = %v, want ErrUserNotFound", err)
	}
}

// OpenRoom creates a public room if needed and joins it; doing it twice is fine.
func TestOpenRoom(t *testing.T) {
	svc := newTest()
	alice, eve, _ := setup(t, svc)
	ctx := t.Context()

	lobby, err := svc.OpenRoom(ctx, alice, "lobby")
	if err != nil {
		t.Fatal(err)
	}
	if lobby.Private || !lobby.IsMember(alice) {
		t.Errorf("lobby: private=%v alice member=%v; want public with alice in it", lobby.Private, lobby.IsMember(alice))
	}
	again, err := svc.OpenRoom(ctx, eve, "LOBBY") // names match like uniqueness does
	if err != nil || again.ID != lobby.ID || !lobby.IsMember(eve) {
		t.Errorf("OpenRoom(LOBBY) = %v, %v; want the same lobby with eve in it", again, err)
	}
	if _, err := svc.OpenRoom(ctx, alice, "lobby"); err != nil {
		t.Errorf("opening a room you're in = %v, want nil", err)
	}
	if _, err := svc.OpenRoom(ctx, alice, "x"); err == nil {
		t.Error("OpenRoom with a bad name worked")
	}
}

func TestChatName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Ace", "Ace"},
		{"Big Ace 99", "Big_Ace_99"},
		{"<Ace>!!", "Ace"},
		{"x", "player"},      // too short
		{"", "player"},       // empty
		{"pаypal", "player"}, // mixes Latin and Cyrillic
		{"ThisNameIsWayTooLongForChat", "ThisNameIsWayTooLong"}, // cut to 20
	}
	for _, tt := range tests {
		if got := chatName(tt.in); got != tt.want {
			t.Errorf("chatName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if s := withSuffix("ThisNameIsWayTooLong"); len([]rune(s)) != chat.MaxNameLen || chat.ValidateName(s) != nil {
		t.Errorf("withSuffix of a 20-character name = %q, want a valid 20-character name", s)
	}
}

func TestGameSessionAndMatchRoom(t *testing.T) {
	svc := newTest()
	ctx := t.Context()

	ace, token, err := svc.GameSession(ctx, "steam:1001", "Ace")
	if err != nil || ace.Name != "Ace" || token == "" {
		t.Fatalf("GameSession = %v, %q, %v; want Ace and a token", ace, token, err)
	}
	if u, err := svc.Authenticate(ctx, token); err != nil || u.ID != ace.ID {
		t.Errorf("the game-issued token doesn't log in as Ace: %v, %v", u, err)
	}

	// The same player again: same user, new session, first name kept.
	again, token2, err := svc.GameSession(ctx, "steam:1001", "Ace The Great")
	if err != nil || again.ID != ace.ID || again.Name != "Ace" || token2 == token {
		t.Errorf("returning player = %v, %v; want the same Ace with a new token", again, err)
	}

	// Another player who wants the same name gets a suffix.
	other, _, err := svc.GameSession(ctx, "steam:2002", "ace")
	if err != nil || other.ID == ace.ID || !strings.HasPrefix(other.Name, "ace-") {
		t.Errorf("second Ace = %v, %v; want a different user named ace-xxxx", other, err)
	}

	// Game players have no password: logging in with one fails.
	if _, _, err := svc.Login(ctx, "Ace", ""); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("password login for a game player = %v, want ErrBadCredentials", err)
	}

	room, err := svc.GameMatchRoom(ctx, "match-77", 0, []string{"steam:1001", "steam:2002"})
	if err != nil || !room.Private || !room.IsMember(ace.ID) || !room.IsMember(other.ID) {
		t.Errorf("GameMatchRoom = %v, %v; want a private room with both players", room, err)
	}
	if _, err := svc.GameMatchRoom(ctx, "match-78", 0, []string{"steam:9999"}); !errors.Is(err, chat.ErrUserNotFound) {
		t.Errorf("a player who never signed in = %v, want ErrUserNotFound", err)
	}
	if _, _, err := svc.GameSession(ctx, "", "Ace"); err == nil {
		t.Error("an empty player ID was accepted")
	}
}
