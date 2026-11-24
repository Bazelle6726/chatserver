package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"chatserver/internal/chat"
	"chatserver/internal/pb"
	"chatserver/internal/pbconv"
)

// sessionCookie holds the browser's session token. The web app never sees
// the token itself:
//   - HttpOnly: page scripts can't read it, so an XSS bug can't steal it
//   - SameSite=Strict: the browser only sends it on requests from this site,
//     so another site can't make a logged-in request for you (CSRF)
//   - Secure when the request came over HTTPS
//
// Other clients (curl, the CLI tools) send the token in an
// "Authorization: Bearer <token>" header instead.
const sessionCookie = "chat_session"

// credentialsReq is the body for POST /users and POST /sessions.
type credentialsReq struct {
	Name     string `json:"name" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// authDTO is what signing up or logging in returns.
type authDTO struct {
	User  userDTO `json:"user"`
	Token string  `json:"token"`
}

// POST /users  {"name": "alice", "password": "..."} - sign up, and log in.
func (a *API) createUser(c *gin.Context) {
	var req credentialsReq
	msg := &pb.CreateUserRequest{}
	fill := func() { req.Name, req.Password = msg.GetName(), msg.GetPassword() }
	if err := bindBody(c, &req, msg, fill); err != nil {
		writeError(c, err)
		return
	}
	user, token, err := a.svc.Register(c.Request.Context(), req.Name, req.Password)
	if err != nil {
		writeError(c, err)
		return
	}
	a.loggedIn(c, http.StatusCreated, user, token)
}

// POST /sessions  {"name": "alice", "password": "..."} - log in.
func (a *API) login(c *gin.Context) {
	var req credentialsReq
	msg := &pb.LoginRequest{}
	fill := func() { req.Name, req.Password = msg.GetName(), msg.GetPassword() }
	if err := bindBody(c, &req, msg, fill); err != nil {
		writeError(c, err)
		return
	}
	user, token, err := a.svc.Login(c.Request.Context(), req.Name, req.Password)
	if err != nil {
		writeError(c, err)
		return
	}
	a.loggedIn(c, http.StatusOK, user, token)
}

// loggedIn sets the session cookie and replies with the user and token.
func (a *API) loggedIn(c *gin.Context, code int, user *chat.User, token string) {
	a.setSessionCookie(c, token, a.svc.SessionTTL())
	respond(c, code, authDTO{User: toUserDTO(user), Token: token},
		&pb.AuthResponse{User: pbconv.User(user), Token: token})
}

// DELETE /sessions (logged in) - log out: end the session and clear the cookie.
func (a *API) logout(c *gin.Context) {
	a.svc.Logout(c.GetString(tokenKey))
	a.setSessionCookie(c, "", -1) // a negative age tells the browser to delete it
	c.Status(http.StatusNoContent)
}

func (a *API) setSessionCookie(c *gin.Context, token string, ttl time.Duration) {
	c.SetSameSite(http.SameSiteStrictMode)
	maxAge := int(ttl.Seconds())
	if ttl < 0 {
		maxAge = -1
	}
	c.SetCookie(sessionCookie, token, maxAge, "/", "", c.Request.TLS != nil, true)
}

// sessionToken finds the request's session token: the Authorization header
// if there is one, else the cookie.
func sessionToken(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); h != "" {
		token, ok := strings.CutPrefix(h, "Bearer ")
		if !ok {
			return ""
		}
		return strings.TrimSpace(token)
	}
	token, _ := c.Cookie(sessionCookie)
	return token
}

// GET /me (logged in) - who am I? The web app asks this on load to find out
// whether its cookie still holds a session.
func (a *API) me(c *gin.Context) {
	user := currentUser(c)
	respond(c, http.StatusOK, toUserDTO(user), pbconv.User(user))
}

// GET /users/:id (logged in) - look a user up, e.g. to show an author's name.
func (a *API) getUser(c *gin.Context) {
	user, err := a.svc.GetUser(c.Request.Context(), chat.UserID(c.Param("id")))
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusOK, toUserDTO(user), pbconv.User(user))
}
