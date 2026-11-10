// Package api is the HTTP layer: it turns HTTP requests into calls on the
// Store and turns the results back into JSON. It contains no chat logic of
// its own - that all lives in the chat and store packages.
package api

import (
	"errors"
	"io/fs"

	"github.com/gin-gonic/gin"

	"chatserver/internal/chat"
	"chatserver/internal/hub"
	"chatserver/internal/moderation"
	"chatserver/internal/store"
)

// API holds what the handlers need. Handlers are methods on *API so they can
// reach the store without global variables.
type API struct {
	store   store.Store
	limiter *Limiter

	// Stage 3: live delivery. Both are optional (nil in most tests).
	hub     *hub.Hub             // who's watching which room
	publish func(m chat.Message) // called after every new message

	// Stage 7: the banned-word list and its admin API. Optional (nil = no
	// /admin routes).
	words      *moderation.List
	wordsPath  string // where changes are saved ("" = don't save)
	adminToken string // the X-Admin-Token admins must send ("" = admin API off)

	// Stage 7: the browser app's files (nil = no web app).
	web fs.FS
}

// New builds the API. perSec is the rate limit per user per second.
func New(s store.Store, perSec int) *API {
	return &API{store: s, limiter: NewLimiter(perSec, nil)}
}

// WithHub turns on live delivery: publish is called with every new message
// (in main it's the worker pool's Submit), and h serves the /stream endpoint.
// It returns a so calls can be chained: api.New(s, 20).WithHub(h, pool.Submit)
func (a *API) WithHub(h *hub.Hub, publish func(chat.Message)) *API {
	a.hub = h
	a.publish = publish
	return a
}

// WithModeration turns on the admin API for the banned-word list. Changes are
// saved to path (unless it's ""), and every admin request must carry an
// X-Admin-Token header equal to token. An empty token turns the admin API
// off: its routes answer 404.
func (a *API) WithModeration(words *moderation.List, path, token string) *API {
	a.words = words
	a.wordsPath = path
	a.adminToken = token
	return a
}

// WithWeb serves the browser app from fsys, which should hold index.html at
// its root (in main: the contents of web/dist). Any GET that no API route
// matches is looked up there.
func (a *API) WithWeb(fsys fs.FS) *API {
	a.web = fsys
	return a
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
