// Command server runs the chat server on http://localhost:8080.
//
// Press Ctrl+C to stop it. It shuts down gracefully: in-flight requests
// finish, live streams close, and every queued message is delivered first.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"time"

	"chatserver/internal/api"
	"chatserver/internal/chat"
	"chatserver/internal/hub"
	"chatserver/internal/store"
)

func main() {
	// ctx is cancelled when you press Ctrl+C. Everything long-running watches it.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	s := store.NewMemoryStore()

	// The hub: one goroutine that knows who's watching which room.
	h := hub.NewHub(64)
	hubDone := make(chan struct{})
	go func() {
		h.Run(ctx)
		close(hubDone)
	}()

	// The moderation pool: workers filter banned words, then publish to the hub.
	// mod.IsBanned is a "method value": a function bound to mod.
	mod := chat.Admin{User: &chat.User{Name: "moderator", Role: chat.RoleAdmin}, BannedWords: []string{"spam", "scam"}}
	pool := hub.NewPool(mod.IsBanned, h.Publish, 256)
	pool.Start(runtime.NumCPU())

	a := api.New(s, 20).WithHub(h, pool.Submit)
	srv := &http.Server{Addr: ":8080", Handler: a.Router()}

	go func() {
		log.Println("chat server listening on http://localhost:8080")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-ctx.Done() // wait for Ctrl+C
	log.Println("shutting down...")

	// The ORDER matters:
	// 1. The hub stops (it watches ctx) and closes every client's outbox,
	//    so every /stream handler returns.
	<-hubDone
	// 2. Stop accepting requests and wait (up to 5s) for in-flight ones.
	//    After this no handler can call pool.Submit any more.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Println("shutdown:", err)
	}
	// 3. Only now stop the pool: closing its jobs channel earlier could make a
	//    late request panic with "send on closed channel".
	pool.Stop()

	log.Printf("bye (dropped %d slow clients)", h.Dropped())
}
