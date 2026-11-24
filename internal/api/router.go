package api

import "github.com/gin-gonic/gin"

// Router builds the Gin engine with every route and middleware.
//
// Recovery is outermost: if any handler panics, the client gets a 500 instead
// of the whole server crashing. Logger prints one line per request.
func (a *API) Router() *gin.Engine {
	r := gin.New()
	// Trust no proxies: c.ClientIP() is then the real peer address. By default
	// Gin believes any X-Forwarded-For header, so a client could claim a new
	// IP on every request and dodge the per-IP admin lockout. Behind a real
	// reverse proxy, list its address here instead.
	if err := r.SetTrustedProxies(nil); err != nil {
		panic(err) // can't happen with nil
	}
	r.Use(gin.Recovery(), gin.Logger(), LimitBody(MaxBodyBytes))
	//Public (anyone can call these):
	r.POST("/users", a.createUser)
	r.GET("/users/:id", a.getUser)
	// and registered on authed instead of r:
	auth := r.Group("/", Auth(a.svc), RateLimit(a.limiter))
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
		failures := NewLimiterPer(adminMaxFailures, adminFailureWindow, nil)
		admin := r.Group("/admin", AdminOnly(a.adminToken, failures))
		admin.GET("/banned-words", a.listBannedWords)
		admin.POST("/banned-words", a.addBannedWord)
		admin.DELETE("/banned-words/:word", a.removeBannedWord)
	}

	return r
}
