// Package ratelimit limits how often each key (a user ID, or an IP address)
// may do something, using a token bucket per key.
//
// A token bucket holds up to burst tokens and refills at perSecond tokens a
// second. Each event takes a token; with none left, the event is refused.
// So a client can make a short burst of requests, but over time no more than
// perSecond. That's smoother than a fixed "N per second" window, which lets
// a client send 2N requests in a moment by straddling the window boundary.
//
// The buckets themselves come from golang.org/x/time/rate. This package adds
// one bucket per key, and forgets keys that have gone quiet, so the map
// can't grow forever.
package ratelimit

import (
	"errors"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ErrLimited is returned (by package service) when a user acts too often.
// HTTP answers it with 429, gRPC with ResourceExhausted.
var ErrLimited = errors.New("too many requests, slow down")

// Limiter gives each key its own token bucket. A nil *Limiter allows
// everything, so "no limit" is just a nil field.
type Limiter struct {
	mu        sync.Mutex
	limit     rate.Limit
	burst     int
	idle      time.Duration // forget a key unused for this long
	now       func() time.Time
	keys      map[string]*entry
	lastSweep time.Time
}

type entry struct {
	bucket   *rate.Limiter
	lastSeen time.Time
}

// Option configures a Limiter.
type Option func(*Limiter)

// WithClock replaces time.Now, so tests can control time.
func WithClock(now func() time.Time) Option {
	return func(l *Limiter) { l.now = now }
}

// New returns a limiter allowing each key perSecond events a second, in
// bursts of up to burst. perSecond <= 0 means no limit: New returns nil.
func New(perSecond float64, burst int, opts ...Option) *Limiter {
	if perSecond <= 0 {
		return nil
	}
	burst = max(burst, 1)
	l := &Limiter{
		limit: rate.Limit(perSecond),
		burst: burst,
		now:   time.Now,
		keys:  make(map[string]*entry),
	}
	for _, opt := range opts {
		opt(l)
	}
	// A bucket left alone this long has refilled completely, and a full
	// bucket is the same as a brand new one. So forgetting it loses nothing.
	l.idle = max(time.Duration(float64(burst)/perSecond*float64(time.Second)), time.Second)
	l.lastSweep = l.now()
	return l
}

// Per is a convenience for limits stated per minute, hour, etc.:
// Per(5, time.Minute) is 5 events a minute.
func Per(n int, d time.Duration) float64 {
	return float64(n) / d.Seconds()
}

// bucket returns key's bucket, creating it if needed. l.mu must be held.
func (l *Limiter) bucket(key string, now time.Time) *rate.Limiter {
	l.sweep(now)
	e := l.keys[key]
	if e == nil {
		e = &entry{bucket: rate.NewLimiter(l.limit, l.burst)}
		l.keys[key] = e
	}
	e.lastSeen = now
	return e.bucket
}

// sweep forgets keys idle for longer than l.idle. It runs at most once per
// idle period, so its cost is spread thin. l.mu must be held.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < l.idle {
		return
	}
	l.lastSweep = now
	for key, e := range l.keys {
		if now.Sub(e.lastSeen) > l.idle {
			delete(l.keys, key)
		}
	}
}

// Allow records one event for key and reports whether it's within the limit.
func (l *Limiter) Allow(key string) bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	return l.bucket(key, now).AllowN(now, 1)
}

// Blocked reports whether key has no tokens left right now, WITHOUT using
// one. The admin check uses it to refuse an IP that has guessed wrong too
// often, while only the wrong guesses (recorded with Allow) use tokens.
func (l *Limiter) Blocked(key string) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.keys[key]
	return e != nil && e.bucket.TokensAt(l.now()) < 1
}

// Len reports how many keys the limiter is tracking (for tests).
func (l *Limiter) Len() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.keys)
}

// Limits groups the limits the two front doors apply, so main can set them
// up once and hand the same ones to the HTTP and the gRPC API. Any of them
// may be nil, meaning no limit.
type Limits struct {
	PerUser *Limiter // every request from a logged-in user, by user ID
	PerIP   *Limiter // every request without a login (sign-up, sign-in, user lookup), by IP
	SignUps *Limiter // creating accounts, by IP
	Logins  *Limiter // login attempts, by IP: slows down password guessing
}
