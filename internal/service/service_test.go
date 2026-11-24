package service

import (
	"errors"
	"testing"
	"time"

	"chatserver/internal/chat"
	"chatserver/internal/hub"
	"chatserver/internal/moderation"
	"chatserver/internal/ratelimit"
	"chatserver/internal/store"
)

// setup returns a service with users alice and eve, and a room "general"
// that only alice has joined.
func setup(t *testing.T, svc *Service) (alice, eve chat.UserID, room chat.RoomID) {
	t.Helper()
	ctx := t.Context()
	a, err := svc.CreateUser(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	e, err := svc.CreateUser(ctx, "eve")
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
	svc := New(store.NewMemoryStore())
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
	svc := New(store.NewMemoryStore()).WithModeration(moderation.New("scam").IsBanned)
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
	svc := New(store.NewMemoryStore()).WithHub(nil, func(m chat.Message) { published = append(published, m) })
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
	if _, _, err := New(store.NewMemoryStore()).Watch(t.Context(), "u-1", "r-1"); !errors.Is(err, ErrLiveOff) {
		t.Errorf("Watch without a hub = %v, want ErrLiveOff", err)
	}

	h := hub.NewHub(4)
	go h.Run(t.Context())
	svc := New(store.NewMemoryStore()).WithHub(h, h.Publish)
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
	svc := New(store.NewMemoryStore()).WithPostLimit(ratelimit.New(1, 3)) // bursts of 3
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
