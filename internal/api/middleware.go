package api

import (
	"chatserver/internal/chat"
	"chatserver/internal/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Auth identifies the caller from the X-User-ID header.
//
// It's a function that RETURNS the middleware: a closure that captures s.
func Auth(svc *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-User-ID")
		if id == "" {
			id = c.Query("user") // for EventSource, which can't set headers
		}
		user, err := svc.GetUser(c.Request.Context(), chat.UserID(id))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid user ID"})
			return
		}
		c.Set(userKey, user) // for currentUser in later handlers
		c.Next()
	}
}

// MaxBodyBytes is the largest request body any route accepts. The biggest
// legitimate body is a 500-character message, a few KB at most.
const MaxBodyBytes = 64 << 10 // 64 KiB

// LimitBody stops a client from sending a huge body to use up the server's
// memory. A body that says up front it's too big is refused straight away;
// one that doesn't (chunked) is cut off by http.MaxBytesReader as it's read,
// and the handler's bind fails with *http.MaxBytesError (see bodyError).
func LimitBody(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > n {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
			return
		}
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		}
		c.Next()
	}
}

// RateLimit stops a user who makes too many requests per second.
// It must run AFTER Auth, because it needs to know who the user is.
func RateLimit(l *Limiter) gin.HandlerFunc {
	return func(c *gin.Context) { // get the user
		user := currentUser(c)
		if !l.Allow(string(user.ID)) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "slow down"})
			return
		}
		c.Next()
		// The lock lives inside Allow, so it's held for a moment, not for the whole
		// request. Holding a lock across c.Next() would make every user wait for the
		// slowest request.
	}
}
