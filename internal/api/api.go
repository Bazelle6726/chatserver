// Package api is the HTTP layer: it turns HTTP requests into calls on the
// service and turns the results back into JSON (or protobuf). It contains no
// chat rules of its own: those all live in package service, which the gRPC
// API shares.
package api

import (
	"errors"
	"io/fs"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"chatserver/internal/chat"
	"chatserver/internal/moderation"
	"chatserver/internal/ratelimit"
	"chatserver/internal/service"
)

// API holds what the handlers need. Handlers are methods on *API so they can
// reach the service without global variables.
type API struct {
	svc    *service.Service
	limits ratelimit.Limits

	// The banned-word list and its admin API. Optional (nil = no /admin
	// routes). Hiding banned words from readers is the service's job; this
	// is only for changing the list.
	words      *moderation.List
	wordsPath  string // where changes are saved ("" = don't save)
	adminToken string // the X-Admin-Token admins must send ("" = admin API off)

	// The browser app's files (nil = no web app).
	web fs.FS
}

// New builds the API on top of svc, applying limits to every request (see
// ratelimit.Limits; a zero Limits means no limits, handy in tests).
func New(svc *service.Service, limits ratelimit.Limits) *API {
	return &API{svc: svc, limits: limits}
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
//
// A 500 is the server's own fault, and its error can hold details a client
// has no business seeing (file paths, internal state). So the client gets a
// plain "internal error" and the real one goes to the server log.
func writeError(c *gin.Context, err error) {
	status := statusFor(err)
	if status == http.StatusInternalServerError {
		log.Printf("internal error: %s %s: %v", c.Request.Method, c.FullPath(), err)
		c.AbortWithStatusJSON(status, gin.H{"error": "internal error"})
		return
	}
	body := gin.H{"error": err.Error()}
	var ve *chat.ValidationError
	if errors.As(err, &ve) {
		body["field"] = ve.Field
	}
	c.AbortWithStatusJSON(status, body)
}

// Where the Auth middleware stores the logged-in user and their token.
const (
	userKey  = "user"
	tokenKey = "token"
)

// currentUser returns the user that Auth put on the context.
// Only call it in handlers that run after Auth.
func currentUser(c *gin.Context) *chat.User {
	return c.MustGet(userKey).(*chat.User)
}
