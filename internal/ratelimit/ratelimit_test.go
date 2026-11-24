package ratelimit

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// fakeClock is a clock the test moves by hand.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time      { return c.t }
func (c *fakeClock) add(d time.Duration) { c.t = c.t.Add(d) }
func newClock() *fakeClock               { return &fakeClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)} }
func newTest(perSec float64, burst int, c *fakeClock) *Limiter {
	return New(perSec, burst, WithClock(c.now))
}

func TestTokenBucket(t *testing.T) {
	c := newClock()
	l := newTest(2, 3, c) // 2 a second, bursts of 3

	for i := range 3 {
		if !l.Allow("u-1") {
			t.Fatalf("event %d of the first burst refused", i+1)
		}
	}
	if l.Allow("u-1") {
		t.Error("a 4th event straight away should be refused: the bucket is empty")
	}
	if !l.Allow("u-2") {
		t.Error("each key has its own bucket: u-2 should be allowed")
	}

	c.add(500 * time.Millisecond) // refills 1 token at 2 a second
	if !l.Allow("u-1") {
		t.Error("after half a second one token is back")
	}
	if l.Allow("u-1") {
		t.Error("...but only one")
	}
}

func TestBlockedDoesNotUseTokens(t *testing.T) {
	c := newClock()
	l := newTest(Per(5, time.Minute), 5, c)
	for range 4 {
		l.Allow("1.2.3.4")
	}
	for range 10 {
		if l.Blocked("1.2.3.4") {
			t.Fatal("blocked after 4 of 5: Blocked must not use tokens")
		}
	}
	l.Allow("1.2.3.4")
	if !l.Blocked("1.2.3.4") {
		t.Error("not blocked after 5 of 5")
	}
	if l.Blocked("5.6.7.8") {
		t.Error("an unseen key can't be blocked")
	}
	c.add(12 * time.Second) // 5 a minute = one token every 12s
	if l.Blocked("1.2.3.4") {
		t.Error("still blocked after a token came back")
	}
}

// Keys that go quiet are forgotten, so the map can't grow forever.
func TestIdleKeysAreForgotten(t *testing.T) {
	c := newClock()
	l := newTest(10, 10, c) // refills completely in 1s
	for i := range 100 {
		l.Allow(fmt.Sprint("ip-", i))
	}
	if l.Len() != 100 {
		t.Fatalf("Len = %d, want 100", l.Len())
	}
	c.add(2 * time.Second)
	l.Allow("ip-new") // any call may sweep
	if l.Len() != 1 {
		t.Errorf("after everyone went quiet, Len = %d, want 1 (just ip-new)", l.Len())
	}
}

func TestNilMeansNoLimit(t *testing.T) {
	var l *Limiter = New(0, 10)
	if l != nil {
		t.Fatal("New(0, ...) should return nil: no limit")
	}
	for range 1000 {
		if !l.Allow("x") {
			t.Fatal("a nil limiter refused an event")
		}
	}
	if l.Blocked("x") || l.Len() != 0 {
		t.Error("a nil limiter is never blocked and tracks nothing")
	}
}

// Run with -race: many goroutines share one limiter.
func TestConcurrentUse(t *testing.T) {
	l := New(1000, 1000)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 500 {
				l.Allow(fmt.Sprint(i, "-", j%10))
				l.Blocked(fmt.Sprint(i))
			}
		}()
	}
	wg.Wait()
}
