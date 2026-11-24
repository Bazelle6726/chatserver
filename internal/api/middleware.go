package api

import (
	"chatserver/internal/chat"
	"chatserver/internal/ratelimit"
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

// RateLimit stops a user who makes too many requests. It must run AFTER
// Auth, because it needs to know who the user is. A nil limiter allows all.
func RateLimit(l *ratelimit.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.Allow(string(currentUser(c).ID)) {
			tooMany(c)
			return
		}
		c.Next()
	}
}

// LimitByIP stops a client address that makes too many requests. It's for
// routes without a login, where there's no user to limit. c.ClientIP() is
// the real peer address, since Router trusts no proxy headers.
func LimitByIP(l *ratelimit.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.Allow(c.ClientIP()) {
			tooMany(c)
			return
		}
		c.Next()
	}
}

func tooMany(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": ratelimit.ErrLimited.Error()})
}
