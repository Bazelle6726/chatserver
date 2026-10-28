package api

import (
	"sync"
	"time"

	"chatserver/internal/chat"
)

// Limiter allows each user at most perSec requests per one-second window.
//
// Gin runs each request in its own goroutine, so several requests can call
// Allow at the same moment. The map must be protected by the mutex.
type Limiter struct {
	mu      sync.Mutex
	perSec  int
	now     func() time.Time // a function field, so tests can fake the clock
	windows map[chat.UserID]*window
}

type window struct {
	start time.Time
	count int
}

// NewLimiter builds a limiter. Pass nil for now to use the real clock.
func NewLimiter(perSec int, now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{perSec: perSec, now: now, windows: make(map[chat.UserID]*window)}
}

// Allow reports whether user id may make another request right now.
//
// TODO(25): a "fixed window" counter:
//   - lock l.mu, and defer the unlock
//   - look up the user's window; if there isn't one, OR its start is a
//     second or more before l.now(), start a fresh window at l.now()
//   - add 1 to the count
//   - return whether the count is still <= l.perSec
//
// Use l.now(), never time.Now() directly, or the tests can't control time.
func (l *Limiter) Allow(id chat.UserID) bool {
	return true
}
