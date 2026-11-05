package chat

import (
	"fmt"
	"testing"
)

// Benchmarks for the room's history. Run with:
//
//	go test ./internal/chat -run XXX -bench History -benchmem
//
// Each runs on a room whose history is already FULL, the state a busy room
// is in all day, for a small and a large history size.

func fullRoom(b *testing.B, max int) *Room {
	b.Helper()
	r := NewRoom("r-1", "bench", max)
	if err := r.Join("u-1"); err != nil {
		b.Fatal(err)
	}
	for i := range max {
		if err := r.AddMessage(msg(i, "u-1")); err != nil {
			b.Fatal(err)
		}
	}
	return r
}

func BenchmarkHistoryAdd(b *testing.B) {
	for _, max := range []int{100, 1000} {
		b.Run(fmt.Sprintf("max=%d", max), func(b *testing.B) {
			r := fullRoom(b, max)
			m := msg(0, "u-1")
			for b.Loop() {
				if err := r.AddMessage(m); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkHistoryLastN(b *testing.B) {
	for _, max := range []int{100, 1000} {
		b.Run(fmt.Sprintf("max=%d", max), func(b *testing.B) {
			r := fullRoom(b, max)
			for b.Loop() {
				_ = r.LastN(50)
			}
		})
	}
}
