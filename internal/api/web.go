package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// serveWeb serves the browser app for every GET that no API route matches:
// "/" gives index.html, "/assets/index-abc123.js" gives that file, and so on.
//
// Why NoRoute and not r.GET("/*filepath", ...)? A catch-all route would
// clash with /users, /rooms and the rest. NoRoute only runs when nothing
// else matched, so the API always wins.
//
// Check: go test ./internal/api -run WebApp -v
func (a *API) serveWeb(r *gin.Engine) {
	files := http.FileServer(http.FS(a.web))
	r.NoRoute(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		files.ServeHTTP(c.Writer, c.Request)
	})
}
