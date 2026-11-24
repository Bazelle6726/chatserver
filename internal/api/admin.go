package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"

	"chatserver/internal/ratelimit"
)

const adminTokenHeader = "X-Admin-Token"

// maxWordLen is the longest banned word the admin API accepts.
const maxWordLen = 50

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
	c.JSON(http.StatusOK, gin.H{"words": a.words.Words()})
}

type bannedWordReq struct {
	Word string `json:"word" binding:"required"`
}

// validWord checks that w could ever match. IsBanned splits messages into runs
// of letters and digits, so a "word" with a space or symbol in it never would.
func validWord(w string) error {
	if w == "" || len([]rune(w)) > maxWordLen {
		return invalid("word", "must be 1-50 characters")
	}
	for _, r := range w {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return invalid("word", "must be a single word of letters and digits")
		}
	}
	return nil
}

// POST /admin/banned-words  {"word": "fraud"}
func (a *API) addBannedWord(c *gin.Context) {
	var req bannedWordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, bodyError(err))
		return
	}
	word := strings.ToLower(strings.TrimSpace(req.Word))
	if err := validWord(word); err != nil {
		writeError(c, err)
		return
	}
	stored := a.words.Add(word) // folded: "Sc4m" is stored as "scam"
	if err := a.saveWords(); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"word": stored})
}

// DELETE /admin/banned-words/:word
func (a *API) removeBannedWord(c *gin.Context) {
	word := strings.ToLower(strings.TrimSpace(c.Param("word")))
	a.words.Remove(word)
	if err := a.saveWords(); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// saveWords writes the list back to its file, so changes survive a restart.
func (a *API) saveWords() error {
	if a.wordsPath == "" {
		return nil
	}
	return a.words.Save(a.wordsPath)
}
