package chat

import (
	"fmt"
	"slices"
	"testing"
)

// LastN's doc promises: "returns up to n of the newest messages, oldest
// first", from a history that "keeps at most maxHistory messages". The ring
// buffer is easy to get subtly wrong (where "oldest" is after wrapping), so
// compare it with the obvious model, a plain list, after any mix of adds and
// reads.
//
//	go test ./internal/chat -run XXX -fuzz FuzzHistoryKeepsNewest -fuzztime 50s
func FuzzHistoryKeepsNewest(f *testing.F) {
	f.Add(uint8(3), []byte{0, 0, 0, 0, 1, 3, 5, 0, 7})
	f.Add(uint8(0), []byte{0, 1, 0, 1})
	f.Fuzz(func(t *testing.T, capacity uint8, ops []byte) {
		size := int(capacity)%20 + 1
		r := NewRoom("r-1", "general", size)
		if err := r.Join("u-1"); err != nil {
			t.Fatal(err)
		}
		var all []MessageID // every message ever added, oldest first
		for i, op := range ops {
			if op%2 == 0 {
				id := MessageID(fmt.Sprint("m-", i))
				if err := r.AddMessage(Message{Meta: Meta{ID: id}, Author: "u-1", Kind: KindText, Text: "x"}); err != nil {
					t.Fatal(err)
				}
				all = append(all, id)
				continue
			}
			n := int(op/2)%(size+3) - 1 // -1 up to size+1: below, inside and past the cap
			kept := all[max(0, len(all)-size):]
			want := kept[len(kept)-min(max(n, 0), len(kept)):]
			if got := ids(r.LastN(n)); !slices.Equal(got, want) {
				t.Fatalf("history of %d (cap %d), LastN(%d) = %v, want %v", len(all), size, n, got, want)
			}
		}
	})
}
