package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"chatserver/internal/chat"
)

func TestValidatePassword(t *testing.T) {
	for _, ok := range []string{"12345678", "correct horse battery staple", strings.Repeat("a", 72)} {
		if err := ValidatePassword(ok); err != nil {
			t.Errorf("ValidatePassword(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "short", strings.Repeat("a", 73)} {
		var ve *chat.ValidationError
		if err := ValidatePassword(bad); !errors.As(err, &ve) || ve.Field != "password" {
			t.Errorf("ValidatePassword(%q) = %v, want a ValidationError for password", bad, err)
		}
	}
}

func TestHashAndCheck(t *testing.T) {
	hash, err := HashPassword("hunter2hunter2", bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if string(hash) == "hunter2hunter2" {
		t.Fatal("the hash is the password")
	}
	if !CheckPassword(hash, "hunter2hunter2") {
		t.Error("the right password didn't match")
	}
	if CheckPassword(hash, "hunter3hunter3") {
		t.Error("a wrong password matched")
	}
	again, _ := HashPassword("hunter2hunter2", bcrypt.MinCost)
	if string(again) == string(hash) {
		t.Error("two hashes of one password are equal: bcrypt should salt each one")
	}
}

func TestSessions(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := NewSessions(time.Hour, func() time.Time { return now })

	token, err := s.Start("u-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 40 {
		t.Errorf("token %q is too short to be unguessable", token)
	}
	other, _ := s.Start("u-1")
	if other == token {
		t.Error("two sessions got the same token")
	}

	if id, ok := s.Lookup(token); !ok || id != "u-1" {
		t.Errorf("Lookup = %q, %v; want u-1, true", id, ok)
	}
	if _, ok := s.Lookup("not-a-token"); ok {
		t.Error("a made-up token was accepted")
	}
	if _, ok := s.Lookup(""); ok {
		t.Error("an empty token was accepted")
	}

	s.End(token)
	if _, ok := s.Lookup(token); ok {
		t.Error("a token still works after End")
	}
	if _, ok := s.Lookup(other); !ok {
		t.Error("ending one session ended another")
	}

	now = now.Add(time.Hour)
	if _, ok := s.Lookup(other); ok {
		t.Error("a session still works after its TTL")
	}
}

// Expired sessions nobody looks up are swept away eventually.
func TestSessionsSweep(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := NewSessions(time.Minute, func() time.Time { return now })
	for range 50 {
		s.Start("u-1")
	}
	now = now.Add(2 * time.Hour)
	s.Start("u-2") // starting a session may sweep
	if s.Len() != 1 {
		t.Errorf("Len = %d after the old sessions expired, want 1", s.Len())
	}
}
