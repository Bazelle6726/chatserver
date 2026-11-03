package hub

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"chatserver/internal/chat"
)

// collector is a deliver function that records what it receives.
// Several workers call it at once, so it needs a mutex.
type collector struct {
	mu   sync.Mutex
	got  []string
	slow time.Duration
}

func (c *collector) deliver(m chat.Message) {
	time.Sleep(c.slow)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.got = append(c.got, m.Text)
}

func (c *collector) texts() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := slices.Clone(c.got)
	slices.Sort(out)
	return out
}

func banned(text string) bool { return strings.Contains(strings.ToLower(text), "spam") }

func TestPoolFiltersAndDeliversEverything(t *testing.T) {
	col := &collector{}
	p := NewPool(banned, col.deliver, 100)
	p.Start(3)

	var originals []chat.Message
	for i := 0; i < 10; i++ {
		text := fmt.Sprintf("msg %d", i)
		if i%3 == 0 {
			text = fmt.Sprintf("buy SPAM %d", i)
		}
		m := chat.Message{Room: "r-1", Text: text}
		originals = append(originals, m)
		p.Submit(m)
	}

	stopped := make(chan struct{})
	go func() { p.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return: are the workers ranging over jobs, and does Stop close it?")
	}

	got := col.texts()
	want := []string{"[removed]", "[removed]", "[removed]", "[removed]",
		"msg 1", "msg 2", "msg 4", "msg 5", "msg 7", "msg 8"}
	if !slices.Equal(got, want) {
		t.Errorf("delivered (sorted) = %q\nwant %q", got, want)
	}

	// The worker changed its copy. The caller's messages must be untouched.
	if originals[0].Text != "buy SPAM 0" {
		t.Errorf("original message changed to %q: workers should edit a copy", originals[0].Text)
	}
}

func TestPoolStopWaitsForWork(t *testing.T) {
	col := &collector{slow: 20 * time.Millisecond}
	p := NewPool(banned, col.deliver, 100)
	p.Start(2)
	for i := 0; i < 6; i++ {
		p.Submit(chat.Message{Text: fmt.Sprint(i)})
	}

	stopped := make(chan struct{})
	go func() { p.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return")
	}
	if n := len(col.texts()); n != 6 {
		t.Errorf("after Stop, %d of 6 messages delivered: Stop must wait for the workers (wg.Wait)", n)
	}
}

func TestPoolUsesSeveralWorkers(t *testing.T) {
	// 4 messages that each take 100ms: with 4 workers that's ~100ms in total,
	// with 1 worker it would be ~400ms.
	col := &collector{slow: 100 * time.Millisecond}
	p := NewPool(banned, col.deliver, 10)
	p.Start(4)
	start := time.Now()
	for i := 0; i < 4; i++ {
		p.Submit(chat.Message{Text: fmt.Sprint(i)})
	}
	stopped := make(chan struct{})
	go func() { p.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return")
	}
	if d := time.Since(start); d > 300*time.Millisecond {
		t.Errorf("took %v: workers don't seem to run in parallel", d)
	}
	if n := len(col.texts()); n != 4 {
		t.Errorf("delivered %d of 4", n)
	}
}
