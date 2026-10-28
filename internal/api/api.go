// Package api is the HTTP layer: it turns HTTP requests into calls on the
// Store and turns the results back into JSON. It contains no chat logic of
// its own - that all lives in the chat and store packages.
package api

import (
	"errors"

	"github.com/gin-gonic/gin"

	"chatserver/internal/chat"
	"chatserver/internal/store"
)

// API holds what the handlers need. Handlers are methods on *API so they can
// reach the store without global variables.
type API struct {
	store   store.Store
	limiter *Limiter
}

// New builds the API. perSec is the rate limit per user per second.
func New(s store.Store, perSec int) *API {
	return &API{store: s, limiter: NewLimiter(perSec, nil)}
}

// writeError sends an error as JSON with the right status code and stops the
// request. Validation errors also say which field was wrong:
//
//	{"error": "invalid name: too short", "field": "name"}
func writeError(c *gin.Context, err error) {
	body := gin.H{"error": err.Error()}
	var ve *chat.ValidationError
	if errors.As(err, &ve) {
		body["field"] = ve.Field
	}
	c.AbortWithStatusJSON(statusFor(err), body)
}

// userKey is where the Auth middleware stores the logged-in user.
const userKey = "user"

// currentUser returns the user that Auth put on the context.
// Only call it in handlers that run after Auth.
func currentUser(c *gin.Context) *chat.User {
	return c.MustGet(userKey).(*chat.User)
}
