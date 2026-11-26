package api

import (
	"crypto/subtle"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"chatserver/internal/ratelimit"
)

const adminTokenHeader = "X-Admin-Token"

// Wrong admin tokens allowed per IP address: a burst of adminMaxFailures,
// then one more per adminFailureWindow/adminMaxFailures (12s). Without a
// limit, a script could try millions of tokens.
const (
	adminMaxFailures   = 5
	adminFailureWindow = time.Minute
)

// AdminOnly lets a request through only if it carries the right admin token.
func AdminOnly(token string, failures *ratelimit.Limiter) gin.HandlerFunc {
	return RequireKey("admin API", adminTokenHeader, token, failures)
}

// RequireKey lets a request through only if its header carries key. It guards
// the admin API and the game API. An empty key turns the API off (404).
// failures counts wrong keys per client IP; an IP that has used up its
// allowance gets 429 without its key even being checked.
func RequireKey(what, header, key string, failures *ratelimit.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if key == "" {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": what + " is off"})
			return
		}
		ip := c.ClientIP() // the real peer address: see SetTrustedProxies in Router
		if failures.Blocked(ip) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many wrong keys, try again later"})
			return
		}
		if subtle.ConstantTimeCompare([]byte(c.GetHeader(header)), []byte(key)) != 1 {
			failures.Allow(ip) // record the failure
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or wrong " + header})
			return
		}
		c.Next()
	}
}

// GET /admin/banned-words
func (a *API) listBannedWords(c *gin.Context) {
	words, err := a.svc.BannedWords()
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"words": words})
}

type bannedWordReq struct {
	Word string `json:"word" binding:"required"`
}

// POST /admin/banned-words  {"word": "fraud"}
func (a *API) addBannedWord(c *gin.Context) {
	var req bannedWordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, bodyError(err))
		return
	}
	stored, err := a.svc.BanWord(req.Word) // folded: "Sc4m" is stored as "scam"
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"word": stored})
}

// DELETE /admin/banned-words/:word
func (a *API) removeBannedWord(c *gin.Context) {
	if err := a.svc.UnbanWord(c.Param("word")); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
