package api

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"chatserver/internal/ratelimit"
)

// Router builds the Gin engine with every route and middleware.
//
// Recovery is outermost: if any handler panics, the client gets a 500 instead
// of the whole server crashing. RequestLog logs one line per request.
func (a *API) Router() *gin.Engine {
	r := gin.New()
	// Trust no proxies: c.ClientIP() is then the real peer address. By default
	// Gin believes any X-Forwarded-For header, so a client could claim a new
	// IP on every request and dodge the per-IP limits. Behind a real reverse
	// proxy, list its address here instead.
	if err := r.SetTrustedProxies(nil); err != nil {
		panic(err) // can't happen with nil
	}
	r.Use(gin.Recovery(), RequestLog(slog.Default()), LimitBody(MaxBodyBytes))

	// Public: signing up and logging in need no session, so they're limited
	// per IP address, each with a stricter limit of its own on top.
	public := r.Group("/", LimitByIP(a.limits.PerIP))
	public.POST("/users", LimitByIP(a.limits.SignUps), a.createUser)
	public.POST("/sessions", LimitByIP(a.limits.Logins), a.login)
	// The WebSocket logs in with its first frame, so it's public here.
	public.GET("/ws", a.websocket)

	// Logged in: limited per user. RateLimit must come after Auth, which
	// says who the user is.
	auth := r.Group("/", Auth(a.svc), RateLimit(a.limits.PerUser))
	auth.DELETE("/sessions", a.logout)
	auth.GET("/me", a.me)
	auth.GET("/users/:id", a.getUser)
	auth.POST("/rooms", a.createRoom)
	auth.GET("/rooms", a.listRooms)
	auth.POST("/rooms/:id/members", a.joinRoom)
	auth.POST("/rooms/:id/messages", a.postMessage)
	auth.GET("/rooms/:id/messages", a.history)
	auth.GET("/rooms/:id/stream", a.stream)

	// Pass the method itself (a.createUser), don't call it (a.createUser()).
	if a.web != nil {
		a.serveWeb(r)
	}

	if a.words != nil {
		failures := ratelimit.New(ratelimit.Per(adminMaxFailures, adminFailureWindow), adminMaxFailures)
		admin := r.Group("/admin", AdminOnly(a.adminToken, failures))
		admin.GET("/banned-words", a.listBannedWords)
		admin.POST("/banned-words", a.addBannedWord)
		admin.DELETE("/banned-words/:word", a.removeBannedWord)
	}

	// The game API: called by a game's backend, never by players. Not
	// limited per IP, since one backend signs in every player; wrong keys
	// are locked out like wrong admin tokens.
	gameFailures := ratelimit.New(ratelimit.Per(adminMaxFailures, adminFailureWindow), adminMaxFailures)
	game := r.Group("/game", RequireKey("game API", gameKeyHeader, a.gameKey, gameFailures))
	game.POST("/sessions", a.gameSession)
	game.POST("/rooms", a.gameRoom)

	return r
}
