// Command server runs the chat server's HTTP API on http://localhost:8080.
//
// Stop it with Ctrl+C.
package main

import (
	"log"

	"chatserver/internal/api"
	"chatserver/internal/store"
)

func main() {
	s := store.NewMemoryStore() // everything is in memory: restarting clears it
	a := api.New(s, 20)         // 20 requests per second per user

	log.Println("chat server listening on http://localhost:8080")
	if err := a.Router().Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
