package api

import (
	"sync"
	"time"
)

// Limiter allows each key (a user ID, or an IP address) at most n events per
// window, e.g. 20 requests a second, or 5 wrong admin tokens a minute.
//
// Gin runs each request in its own goroutine, so several requests can call
// Allow at the same moment. The map must be protected by the mutex.
type Limiter struct {
	mu      sync.Mutex
	n       int
	per     time.Duration
	now     func() time.Time // a function field, so tests can fake the clock
	windows map[string]*window
}

type window struct {
	start time.Time
	count int
}

// NewLimiter builds a limiter of perSec events per second for each key.
// Pass nil for now to use the real clock.
func NewLimiter(perSec int, now func() time.Time) *Limiter {
	return NewLimiterPer(perSec, time.Second, now)
}

// NewLimiterPer builds a limiter of n events per window of length per.
func NewLimiterPer(n int, per time.Duration, now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{n: n, per: per, now: now, windows: make(map[string]*window)}
}

// Allow records one event for key and reports whether it's within the limit.
//
// "fixed window" counter:
//   - look up the key's window; if there isn't one, OR it started a whole
//     window or more ago, start a fresh one now
//   - add 1 to the count
//   - return whether the count is still <= n
//
// Use l.now(), never time.Now() directly, or the tests can't control time.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	w := l.windows[key]
	if w == nil || l.now().Sub(w.start) >= l.per {
		w = &window{start: l.now()}
		l.windows[key] = w
	}
	w.count++
	return w.count <= l.n
}

// Blocked reports whether key has already used up its events in the current
// window, WITHOUT recording a new one. The admin check uses it to refuse an
// IP that has guessed wrong too often, while counting only the wrong guesses.
func (l *Limiter) Blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	w := l.windows[key]
	return w != nil && l.now().Sub(w.start) < l.per && w.count >= l.n
}
