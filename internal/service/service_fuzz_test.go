package service

import (
	"testing"
	"unicode/utf8"

	"chatserver/internal/chat"
)

// GameSession's doc comment promises: a new player gets a chat user named
// after their display name, made valid, and "if that's taken, a short random
// suffix is added". So two different players with the same display name must
// both be signed in, with valid, different names. A game can't choose its
// players' names to suit the chat server.
//
//	go test ./internal/service -run XXX -fuzz FuzzSameDisplayNameBothSignIn -fuzztime 50s
func FuzzSameDisplayNameBothSignIn(f *testing.F) {
	for _, s := range []string{"Ace", "Big Ace 99", "<Ace>!!", "x", "алиса", "Ωmega", "pаypal", "ThisNameIsWayTooLongForChat"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, display string) {
		if !utf8.ValidString(display) {
			t.Skip()
		}
		svc := newTest()
		ctx := t.Context()
		first, _, err := svc.GameSession(ctx, "player:1", display)
		if err != nil {
			t.Fatalf("first player named %+q refused: %v", display, err)
		}
		second, _, err := svc.GameSession(ctx, "player:2", display)
		if err != nil {
			t.Fatalf("second player named %+q refused (the first got %+q): %v", display, first.Name, err)
		}
		for _, u := range []*chat.User{first, second} {
			if err := chat.ValidateName(u.Name); err != nil {
				t.Errorf("player got invalid name %+q: %v", u.Name, err)
			}
		}
		if first.Name == second.Name {
			t.Errorf("both players got the name %+q", first.Name)
		}
	})
}
