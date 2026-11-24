package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"chatserver/internal/chat"
)

// Run these with the race detector:  go test -race ./internal/store
// Without locks they crash ("concurrent map writes") or report a DATA RACE.

func TestIDGeneratorConcurrent(t *testing.T) {
	next := NewIDGenerator("m")
	const goroutines, each = 50, 100

	var mu sync.Mutex
	seen := make(map[string]bool)
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range each {
				id := next()
				mu.Lock()
				seen[id] = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(seen) != goroutines*each {
		t.Errorf("got %d unique IDs from %d calls: two goroutines received the same ID",
			len(seen), goroutines*each)
	}
}

func TestStoreConcurrentAccess(t *testing.T) {
	s := NewMemoryStore()
	const workers = 20

	done := make(chan struct{})
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				u, err := s.CreateUser(t.Context(), fmt.Sprintf("user%d", i))
				if err != nil {
					t.Errorf("CreateUser: %v", err)
					return
				}
				r, err := s.CreateRoom(t.Context(), fmt.Sprintf("room%d", i), 10)
				if err != nil {
					t.Errorf("CreateRoom: %v", err)
					return
				}
				if err := r.Join(u.ID); err != nil {
					t.Errorf("Join: %v", err)
					return
				}
				for j := 0; j < 10; j++ {
					if _, err := s.PostMessage(t.Context(), r.ID, u.ID, "hello"); err != nil {
						t.Errorf("PostMessage: %v", err)
					}
					_ = mustList(t, s)
					if _, err := s.GetUser(t.Context(), u.ID); err != nil {
						t.Errorf("GetUser: %v", err)
					}
				}
			}()
		}
		wg.Wait()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out: probably a deadlock (a method locking the store twice)")
	}

	if n := len(mustList(t, s)); n != workers {
		t.Fatalf("ListRooms has %d rooms, want %d (IDs collided or a write was lost)", n, workers)
	}
	ids := make(map[chat.RoomID]bool)
	for _, r := range mustList(t, s) {
		ids[r.ID] = true
	}
	if len(ids) != workers {
		t.Errorf("only %d distinct room IDs for %d rooms", len(ids), workers)
	}
}

func TestDuplicateNamesUnderConcurrency(t *testing.T) {
	// 20 goroutines all try to create "alice" at once. Exactly one may win.
	// Checking for a duplicate and then inserting must happen under ONE lock,
	// or two goroutines can both pass the check before either inserts.
	s := NewMemoryStore()
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.CreateUser(t.Context(), "alice"); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Errorf("%d goroutines created \"alice\", want exactly 1", wins)
	}
}

func mustList(t *testing.T, s *MemoryStore) []*chat.Room {
	t.Helper()
	rooms, err := s.ListRooms(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return rooms
}

// Every method gives up straight away if its context is already cancelled.
func TestCancelledContext(t *testing.T) {
	s := NewMemoryStore()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.CreateUser(ctx, "alice"); !errors.Is(err, context.Canceled) {
		t.Errorf("CreateUser with a cancelled context = %v, want context.Canceled", err)
	}
	if _, err := s.ListRooms(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("ListRooms with a cancelled context = %v, want context.Canceled", err)
	}
	if _, err := s.GetUser(t.Context(), "u-1"); !errors.Is(err, chat.ErrUserNotFound) {
		t.Errorf("CreateUser with a cancelled context must not have stored the user: %v", err)
	}
}
