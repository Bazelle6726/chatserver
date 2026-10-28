package api

import "github.com/gin-gonic/gin"

// Router builds the Gin engine with every route and middleware.
//
// Recovery is outermost: if any handler panics, the client gets a 500 instead
// of the whole server crashing. Logger prints one line per request.
func (a *API) Router() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger())

	// TODO(28): register the routes.
	//
	// Public (anyone can call these):
	//   POST /users              -> a.createUser
	//   GET  /users/:id          -> a.getUser
	//
	// Logged in: make a group whose middleware runs Auth first, then the
	// rate limiter:
	//
	//   authed := r.Group("/", Auth(a.store), RateLimit(a.limiter))
	//
	// and register these on authed instead of r:
	//   POST /rooms                 -> a.createRoom
	//   GET  /rooms                 -> a.listRooms
	//   POST /rooms/:id/members     -> a.joinRoom
	//   POST /rooms/:id/messages    -> a.postMessage
	//   GET  /rooms/:id/messages    -> a.history
	//
	// Pass the method itself (a.createUser), don't call it (a.createUser()).

	return r
}
