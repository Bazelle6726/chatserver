package api

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"chatserver/internal/chat"
)

// RequestLog logs one structured line per request with log/slog, in place of
// gin.Logger's plain text:
//
//	level=INFO msg="http request" method=POST route=/rooms/:id/messages status=201 ms=0.4 ip=127.0.0.1 user=u-3f9c...
//
// It logs the route pattern (/rooms/:id/messages), not the URL, so lines
// group by endpoint and nothing from the query string ends up in the log.
// Server errors (5xx) are logged at Error level, everything else at Info.
func RequestLog(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path // no route matched: a web app file, or a 404
		}
		status := c.Writer.Status()
		attrs := []slog.Attr{
			slog.String("method", c.Request.Method),
			slog.String("route", route),
			slog.Int("status", status),
			slog.Float64("ms", float64(time.Since(start).Microseconds())/1000),
			slog.String("ip", c.ClientIP()),
		}
		if u, ok := c.Get(userKey); ok {
			if user, ok := u.(*chat.User); ok {
				attrs = append(attrs, slog.String("user", string(user.ID)))
			}
		}
		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		}
		logger.LogAttrs(c.Request.Context(), level, "http request", attrs...)
	}
}
