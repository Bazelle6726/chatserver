package store

import (
	"strings"
	"testing"
	"unicode/utf8"

	"chatserver/internal/chat"
)

// docs/API.md promises: "Messages: not blank, at most 500 characters." So
// whatever PostMessage accepts and stores must be at most 500 characters and
// not blank. Every client (the web app, the Unity chat box) is built on that.
//
//	go test ./internal/store -run XXX -fuzz FuzzAcceptedMessageFitsLimit -fuzztime 50s
func FuzzAcceptedMessageFitsLimit(f *testing.F) {
	f.Add("hello", 0)
	f.Add(strings.Repeat("a", chat.MaxMessageLen), 0)
	f.Add("   ", 0)
	f.Fuzz(func(t *testing.T, text string, padding int) {
		if !utf8.ValidString(text) || padding < 0 || padding > 2000 {
			t.Skip()
		}
		text += strings.Repeat(" ", padding) // clients send what the player typed, spaces included
		s := NewMemoryStore()
		ctx := t.Context()
		u, _ := s.CreateUser(ctx, "alice", nil)
		r, _ := s.CreateRoom(ctx, "general", 10, false)
		if err := r.Join(u.ID); err != nil {
			t.Fatal(err)
		}
		m, err := s.PostMessage(ctx, r.ID, u.ID, text)
		if err != nil {
			return // refused: nothing stored, nothing promised
		}
		if n := utf8.RuneCountInString(m.Text); n > chat.MaxMessageLen {
			t.Errorf("stored a %d-character message (the limit is %d): %d characters of text plus %d spaces",
				n, chat.MaxMessageLen, utf8.RuneCountInString(strings.TrimSpace(m.Text)), n-utf8.RuneCountInString(strings.TrimSpace(m.Text)))
		}
		if strings.TrimSpace(m.Text) == "" {
			t.Errorf("stored a blank message %+q", m.Text)
		}
	})
}
