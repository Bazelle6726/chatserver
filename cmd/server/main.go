// Command server will run the chat server. In stage 1 it just exercises the
// store so you can see your code working with `go run ./cmd/server`.
package main

import (
	"errors"
	"fmt"
	"log"

	"chatserver/internal/chat"
	"chatserver/internal/store"
)

func main() {
	var s store.Store = store.NewMemoryStore()

	alice, err := s.CreateUser("alice")
	if err != nil || alice == nil {
		log.Fatalf("create user: %v (finish the TODOs in internal/store first)", err)
	}
	room, err := s.CreateRoom("general", 100)
	if err != nil || room == nil {
		log.Fatalf("create room: %v", err)
	}
	if err := room.Join(alice.ID); err != nil {
		log.Fatalf("join: %v", err)
	}

	for _, text := range []string{"hello gophers", "   "} {
		m, err := s.PostMessage(room.ID, alice.ID, text)
		var ve *chat.ValidationError
		switch {
		case errors.As(err, &ve):
			fmt.Printf("rejected: %v\n", ve)
		case err != nil:
			log.Fatalf("post: %v", err)
		default:
			fmt.Printf("[%s] %s (%s): %s\n", room.Name, alice.Name, alice.Role, m.Text)
		}
	}

	if _, err := s.GetRoom("r-404"); errors.Is(err, chat.ErrRoomNotFound) {
		fmt.Println("lookup failed as expected:", err)
	}
}
