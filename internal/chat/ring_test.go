package chat

import (
	"fmt"
	"testing"
)

// TODO 73: the history as a ring buffer. Whatever the storage, LastN must
// always return the newest messages, oldest first.
func TestHistoryWrapsAround(t *testing.T) {
	const size = 4
	r := newTestRoom(t, size)
	_ = r.Join("u-1")

	// Add 1..11, checking LastN after every message: this covers before the
	// buffer is full, exactly full, and wrapping round several times.
	for i := 1; i <= 11; i++ {
		if err := r.AddMessage(msg(i, "u-1")); err != nil {
			t.Fatal(err)
		}
		for n := 0; n <= size+1; n++ {
			got := ids(r.LastN(n))
			want := []MessageID{}
			for j := max(1, i-min(n, size)+1); j <= i; j++ {
				want = append(want, MessageID(fmt.Sprintf("m-%d", j)))
			}
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatalf("after adding m-1..m-%d, LastN(%d) = %v, want %v", i, n, got, want)
			}
		}
	}
	if cap(r.history) != size {
		t.Errorf("history capacity is %d, want %d: a ring buffer never reallocates", cap(r.history), size)
	}
}

// Adding to a full history must cost the same whatever its size. This
// compares a small and a large history; with copy() the large one is about
// ten times slower.
func TestAddIsConstantTime(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	small := testing.Benchmark(func(b *testing.B) { benchAdd(b, 10) })
	large := testing.Benchmark(func(b *testing.B) { benchAdd(b, 10000) })
	ratio := float64(large.NsPerOp()) / float64(max(small.NsPerOp(), 1))
	t.Logf("AddMessage on a full history: max=10 %d ns, max=10000 %d ns (%.1fx)", small.NsPerOp(), large.NsPerOp(), ratio)
	if ratio > 5 {
		t.Errorf("a 1000x larger history makes AddMessage %.0fx slower: it still copies the whole history", ratio)
	}
}

func benchAdd(b *testing.B, size int) {
	r := fullRoom(b, size)
	m := msg(0, "u-1")
	for b.Loop() {
		_ = r.AddMessage(m)
	}
}
