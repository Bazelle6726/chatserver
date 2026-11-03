package chat

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestRoomConcurrentAccess hits one room from many goroutines at once, the
// way Gin's request goroutines will.
//
// Without locks this fails in one of two ways:
//   - "fatal error: concurrent map writes" (Go detects it and crashes), or
//   - with go test -race: "WARNING: DATA RACE" and the test fails.
//
// If it times out instead, a method probably locks the mutex and then calls
// another method that tries to lock it again (see TODO 36 in the README).
func TestRoomConcurrentAccess(t *testing.T) {
	r := NewRoom("r-1", "general", 50)
	const users = 20

	done := make(chan struct{})
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		for i := 0; i < users; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				id := UserID(fmt.Sprintf("u-%d", i))
				if err := r.Join(id); err != nil {
					t.Errorf("Join(%s) = %v", id, err)
					return
				}
				for j := 0; j < 20; j++ {
					m := Message{Meta: Meta{ID: MessageID(fmt.Sprintf("m-%d-%d", i, j))}, Author: id, Kind: KindText, Text: "hi"}
					if err := r.AddMessage(m); err != nil {
						t.Errorf("AddMessage = %v", err)
					}
					_ = r.LastN(5)
					_ = r.IsMember(id)
					_ = r.MemberCount()
					_ = r.SettingsCopy()
				}
			}()
		}
		wg.Wait()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out: probably a deadlock (a method locking the room twice)")
	}

	if got := r.MemberCount(); got != users {
		t.Errorf("MemberCount = %d, want %d", got, users)
	}
	if got := len(r.LastN(1000)); got != 50 {
		t.Errorf("history holds %d messages, want 50 (the cap)", got)
	}
}
