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
	if _, err := s.CreateUser("alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("bob"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Alice", "ALICE", "ａｌｉｃｅ", "ВОВ" /* all Cyrillic */} {
		if _, err := s.CreateUser(name); !errors.Is(err, chat.ErrAlreadyExists) {
			t.Errorf("CreateUser(%q) = %v, want ErrAlreadyExists", name, err)
		}
	}

	if _, err := s.CreateRoom("general", 10); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRoom("General", 10); !errors.Is(err, chat.ErrAlreadyExists) {
		t.Errorf("CreateRoom(%q) = %v, want ErrAlreadyExists", "General", err)
	}
	if _, err := s.CreateUser("alicia"); err != nil {
		t.Errorf("a genuinely different name was refused: %v", err)
	}
}
