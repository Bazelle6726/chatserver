package api

import (
	"github.com/gin-gonic/gin"

	"chatserver/internal/store"
)

// Auth identifies the caller from the X-User-ID header.
// (A real app would check a password or token; this keeps stage 2 simple.)
//
// It's a function that RETURNS the middleware: a closure that captures s.
//
// TODO(27): inside the returned function:
//   - read the header: c.GetHeader("X-User-ID")
//   - if it's empty, or s.GetUser can't find that user, stop the request with
//     c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "..."})
//     and return
//   - otherwise store the user for later handlers: c.Set(userKey, user)
//   - then call c.Next() to run the rest of the chain
//
// You'll need to import "net/http" and "chatserver/internal/chat".
func Auth(s store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

// RateLimit stops a user who makes too many requests per second.
// It must run AFTER Auth, because it needs to know who the user is.
//
// TODO(26): inside the returned function:
//   - get the user with currentUser(c)
//   - if !l.Allow(user.ID), stop with
//     c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "slow down"})
//     and return
//   - otherwise c.Next()
//
// The lock lives inside Allow, so it's held for a moment, not for the whole
// request. Holding a lock across c.Next() would make every user wait for the
// slowest request.
func RateLimit(l *Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}
