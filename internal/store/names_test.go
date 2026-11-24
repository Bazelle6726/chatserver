package store

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"chatserver/internal/chat"
)

// A name that only LOOKS like a taken one is taken too: different case,
// fullwidth letters, or Cyrillic look-alikes.
func TestLookalikeNamesAreTaken(t *testing.T) {
	s := NewMemoryStore()
	if _, err := s.CreateUser(t.Context(), "alice", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser(t.Context(), "bob", nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Alice", "ALICE", "ａｌｉｃｅ", "ВОВ" /* all Cyrillic */} {
		if _, err := s.CreateUser(t.Context(), name, nil); !errors.Is(err, chat.ErrAlreadyExists) {
			t.Errorf("CreateUser(%q) = %v, want ErrAlreadyExists", name, err)
		}
	}

	if _, err := s.CreateRoom(t.Context(), "general", 10); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRoom(t.Context(), "General", 10); !errors.Is(err, chat.ErrAlreadyExists) {
		t.Errorf("CreateRoom(%q) = %v, want ErrAlreadyExists", "General", err)
	}
	if _, err := s.CreateUser(t.Context(), "alicia", nil); err != nil {
		t.Errorf("a genuinely different name was refused: %v", err)
	}
}

// UserByName matches the same way uniqueness does, and the password hash is
// kept per user.
func TestUserByNameAndPasswordHash(t *testing.T) {
	s := NewMemoryStore()
	ctx := t.Context()
	alice, err := s.CreateUser(ctx, "alice", []byte("hash-a"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alice", "ALICE"} {
		u, err := s.UserByName(ctx, name)
		if err != nil || u.ID != alice.ID {
			t.Errorf("UserByName(%q) = %v, %v; want alice", name, u, err)
		}
	}
	if _, err := s.UserByName(ctx, "nobody"); !errors.Is(err, chat.ErrUserNotFound) {
		t.Errorf("UserByName(nobody) = %v, want ErrUserNotFound", err)
	}
	if h, err := s.PasswordHash(ctx, alice.ID); err != nil || string(h) != "hash-a" {
		t.Errorf("PasswordHash = %q, %v; want hash-a", h, err)
	}
	if _, err := s.PasswordHash(ctx, "u-404"); !errors.Is(err, chat.ErrUserNotFound) {
		t.Errorf("PasswordHash of a missing user = %v, want ErrUserNotFound", err)
	}
}

// User IDs are random, so they can't be guessed or counted.
func TestUserIDsAreRandom(t *testing.T) {
	s := NewMemoryStore()
	seen := map[chat.UserID]bool{}
	for i := range 50 {
		u, err := s.CreateUser(t.Context(), fmt.Sprintf("user%02d", i), nil)
		if err != nil {
			t.Fatal(err)
		}
		if seen[u.ID] {
			t.Fatalf("duplicate ID %s", u.ID)
		}
		seen[u.ID] = true
		if !strings.HasPrefix(string(u.ID), "u-") || len(u.ID) != len("u-")+16 {
			t.Errorf("ID %q, want u- and 16 hex digits", u.ID)
		}
	}
	if seen["u-1"] || seen["u-2"] {
		t.Error("IDs look counted, not random")
	}
}
