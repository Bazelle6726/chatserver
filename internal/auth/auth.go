// Package auth handles passwords and sessions.
//
// Passwords are never stored, only bcrypt hashes of them. bcrypt is slow on
// purpose (and salts each hash), so a stolen list of hashes can't be turned
// back into passwords quickly.
//
// A session is what a user gets for logging in: a random token, which the
// client sends with every request instead of a password. The server keeps
// only a SHA-256 of each token, so even someone who could read the server's
// memory couldn't use the sessions they found there.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"chatserver/internal/chat"
)

// Password rules. bcrypt only looks at the first 72 bytes, so longer
// passwords are refused rather than silently cut short.
const (
	MinPasswordLen = 8
	MaxPasswordLen = 72 // bytes
)

// ValidatePassword checks a new password against the rules.
func ValidatePassword(pw string) error {
	if len([]rune(pw)) < MinPasswordLen {
		return &chat.ValidationError{Field: "password", Reason: "must be at least 8 characters"}
	}
	if len(pw) > MaxPasswordLen {
		return &chat.ValidationError{Field: "password", Reason: "must be at most 72 bytes"}
	}
	return nil
}

// HashPassword returns a bcrypt hash of pw. cost is bcrypt's work factor:
// bcrypt.DefaultCost (10) in production, bcrypt.MinCost (4) to keep tests fast.
func HashPassword(pw string, cost int) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(pw), cost)
}

// CheckPassword reports whether pw matches hash. It takes as long whether
// or not it matches, so its timing gives nothing away.
func CheckPassword(hash []byte, pw string) bool {
	return bcrypt.CompareHashAndPassword(hash, []byte(pw)) == nil
}

// DefaultSessionTTL is how long a login lasts.
const DefaultSessionTTL = 7 * 24 * time.Hour

// Sessions holds the logged-in sessions, in memory. Safe for concurrent use.
type Sessions struct {
	mu        sync.Mutex
	ttl       time.Duration
	now       func() time.Time
	byHash    map[[sha256.Size]byte]session
	lastSweep time.Time
}

type session struct {
	user    chat.UserID
	expires time.Time
}

// NewSessions returns an empty session store whose sessions last ttl.
// now may be nil for the real clock.
func NewSessions(ttl time.Duration, now func() time.Time) *Sessions {
	if now == nil {
		now = time.Now
	}
	return &Sessions{ttl: ttl, now: now, byHash: make(map[[sha256.Size]byte]session), lastSweep: now()}
}

// TTL is how long each session lasts.
func (s *Sessions) TTL() time.Duration { return s.ttl }

// Start creates a session for user and returns its token: 32 random bytes
// (256 bits), so it can't be guessed.
func (s *Sessions) Start(user chat.UserID) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep()
	s.byHash[sha256.Sum256([]byte(token))] = session{user: user, expires: s.now().Add(s.ttl)}
	return token, nil
}

// Lookup returns the user a token belongs to, if it's a live session.
func (s *Sessions) Lookup(token string) (chat.UserID, bool) {
	if token == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := sha256.Sum256([]byte(token))
	sess, ok := s.byHash[key]
	if !ok {
		return "", false
	}
	if !s.now().Before(sess.expires) {
		delete(s.byHash, key)
		return "", false
	}
	return sess.user, true
}

// End logs a session out. Ending an unknown token does nothing.
func (s *Sessions) End(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byHash, sha256.Sum256([]byte(token)))
}

// Len reports how many sessions are stored (for tests).
func (s *Sessions) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.byHash)
}

// sweep drops expired sessions, at most once an hour, so sessions nobody
// ends don't pile up forever. s.mu must be held.
func (s *Sessions) sweep() {
	now := s.now()
	if now.Sub(s.lastSweep) < time.Hour {
		return
	}
	s.lastSweep = now
	for key, sess := range s.byHash {
		if !now.Before(sess.expires) {
			delete(s.byHash, key)
		}
	}
}
