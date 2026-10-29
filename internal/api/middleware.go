package api

import (
	"chatserver/internal/chat"
	"chatserver/internal/store"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Auth identifies the caller from the X-User-ID header.
//
// It's a function that RETURNS the middleware: a closure that captures s.
func Auth(s store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetHeader("X-User-ID") //header is read
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing user ID"})
			return
		} //if it's empty, or s.GetUser can't find that user, the request is stopped
		user, err := s.GetUser(chat.UserID(userID))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid user ID"})
			return
		}
		c.Set("user", user) //user is stored for later handlers
		c.Next()            //runs rest of the chain
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
