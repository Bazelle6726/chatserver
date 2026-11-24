package store

import (
	"errors"
	"testing"

	"chatserver/internal/chat"
)

// A name that only LOOKS like a taken one is taken too: different case,
// fullwidth letters, or Cyrillic look-alikes.
func TestLookalikeNamesAreTaken(t *testing.T) {
	s := NewMemoryStore()
	if _, err := s.CreateUser(t.Context(), "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser(t.Context(), "bob"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Alice", "ALICE", "ａｌｉｃｅ", "ВОВ" /* all Cyrillic */} {
		if _, err := s.CreateUser(t.Context(), name); !errors.Is(err, chat.ErrAlreadyExists) {
			t.Errorf("CreateUser(%q) = %v, want ErrAlreadyExists", name, err)
		}
	}

	if _, err := s.CreateRoom(t.Context(), "general", 10); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRoom(t.Context(), "General", 10); !errors.Is(err, chat.ErrAlreadyExists) {
		t.Errorf("CreateRoom(%q) = %v, want ErrAlreadyExists", "General", err)
	}
	if _, err := s.CreateUser(t.Context(), "alicia"); err != nil {
		t.Errorf("a genuinely different name was refused: %v", err)
	}
}
