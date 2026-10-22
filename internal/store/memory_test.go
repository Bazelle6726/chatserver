package store

import (
	"errors"
	"testing"
	"time"

	"chatserver/internal/chat"
)

func TestIDGenerator(t *testing.T) {
	a := NewIDGenerator("u")
	b := NewIDGenerator("r")
	for _, want := range []string{"u-1", "u-2", "u-3"} {
		if got := a(); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
	if got := b(); got != "r-1" {
		t.Errorf("second generator got %q, want r-1 (each closure has its own counter)", got)
	}
}

var fixed = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func newStore(t *testing.T) *MemoryStore {
	t.Helper()
	s := NewMemoryStore(WithClock(func() time.Time { return fixed }))
	if s.users == nil || s.rooms == nil || s.nextUserID == nil || s.now == nil {
		t.Fatal("NewMemoryStore must set up maps, ID generators and the clock (TODO 16)")
	}
	return s
}

func mustUser(t *testing.T, s *MemoryStore, name string) *chat.User {
	t.Helper()
	u, err := s.CreateUser(name)
	if err != nil || u == nil {
		t.Fatalf("CreateUser(%q) = %v, %v", name, u, err)
	}
	return u
}

func mustRoom(t *testing.T, s *MemoryStore, name string) *chat.Room {
	t.Helper()
	r, err := s.CreateRoom(name, 50)
	if err != nil || r == nil {
		t.Fatalf("CreateRoom(%q) = %v, %v", name, r, err)
	}
	return r
}

func TestOptionsApplied(t *testing.T) {
	s := newStore(t)
	if !s.now().Equal(fixed) {
		t.Error("WithClock option was not applied")
	}
	if NewMemoryStore().now == nil {
		t.Error("default clock should be time.Now")
	}
}

func TestCreateAndGetUser(t *testing.T) {
	s := newStore(t)
	u := mustUser(t, s, "alice")
	if u.ID != "u-1" || u.Role != chat.RoleMember {
		t.Errorf("got ID %q role %v, want u-1 member", u.ID, u.Role)
	}

	got, err := s.GetUser(u.ID)
	if err != nil || got != u {
		t.Errorf("GetUser = %v, %v; want the same *User", got, err)
	}

	if _, err := s.CreateUser("alice"); !errors.Is(err, chat.ErrAlreadyExists) {
		t.Errorf("duplicate CreateUser = %v, want ErrAlreadyExists (wrapped is fine)", err)
	}

	var ve *chat.ValidationError
	if _, err := s.CreateUser("x"); !errors.As(err, &ve) {
		t.Errorf("CreateUser(x) = %v, want *ValidationError", err)
	}

	_, err = s.GetUser("nobody")
	if !errors.Is(err, chat.ErrUserNotFound) {
		t.Errorf("GetUser(nobody) = %v, want ErrUserNotFound", err)
	}
}

func TestCreateAndListRooms(t *testing.T) {
	s := newStore(t)
	for _, n := range []string{"random", "general", "dev"} {
		mustRoom(t, s, n)
	}
	if _, err := s.CreateRoom("dev", 50); !errors.Is(err, chat.ErrAlreadyExists) {
		t.Errorf("duplicate CreateRoom = %v, want ErrAlreadyExists", err)
	}
	if _, err := s.GetRoom("r-404"); !errors.Is(err, chat.ErrRoomNotFound) {
		t.Errorf("GetRoom(r-404) = %v, want ErrRoomNotFound", err)
	}

	rooms := s.ListRooms()
	var names []string
	for _, r := range rooms {
		names = append(names, r.Name)
	}
	want := []string{"dev", "general", "random"}
	if len(names) != len(want) {
		t.Fatalf("ListRooms names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("ListRooms names = %v, want %v (sorted by name)", names, want)
		}
	}
}

func TestPostMessage(t *testing.T) {
	s := newStore(t)
	u := mustUser(t, s, "alice")
	r := mustRoom(t, s, "general")

	if _, err := s.PostMessage(r.ID, u.ID, "hi"); !errors.Is(err, chat.ErrNotMember) {
		t.Errorf("post before joining = %v, want ErrNotMember", err)
	}
	_ = r.Join(u.ID)

	m, err := s.PostMessage(r.ID, u.ID, "hello gophers")
	if err != nil {
		t.Fatalf("PostMessage = %v", err)
	}
	if m.ID == "" || m.Kind != chat.KindText || m.Author != u.ID || m.Room != r.ID || !m.CreatedAt.Equal(fixed) {
		t.Errorf("message fields wrong: %+v", m)
	}
	if hist := r.LastN(1); len(hist) != 1 || hist[0].ID != m.ID {
		t.Error("message was not added to the room's history")
	}

	if _, err := s.PostMessage("r-404", u.ID, "hi"); !errors.Is(err, chat.ErrRoomNotFound) {
		t.Errorf("unknown room = %v, want ErrRoomNotFound", err)
	}
	if _, err := s.PostMessage(r.ID, "u-404", "hi"); !errors.Is(err, chat.ErrUserNotFound) {
		t.Errorf("unknown author = %v, want ErrUserNotFound", err)
	}
	var ve *chat.ValidationError
	if _, err := s.PostMessage(r.ID, u.ID, "   "); !errors.As(err, &ve) {
		t.Errorf("blank text = %v, want *ValidationError", err)
	}
}
