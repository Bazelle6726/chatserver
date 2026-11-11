package api

import (
	"net/http"

	"strings"
	"unicode"

	"crypto/subtle"

	"chatserver/internal/chat"

	"github.com/gin-gonic/gin"
)

const adminTokenHeader = "X-Admin-Token"

// maxWordLen is the longest banned word the admin API accepts.
const maxWordLen = 50

// AdminOnly lets a request through only if it carries the right admin token.
func AdminOnly(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if token == "" {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "admin API is off"})
			return
		}
		if subtle.ConstantTimeCompare([]byte(c.GetHeader(adminTokenHeader)), []byte(token)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or wrong X-Admin-Token"})
			return
		}
		c.Next()
	}
}

// GET /admin/banned-words
//
// TODO(76): reply 200 with gin.H{"words": a.words.Words()}
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
		writeError(c, invalid("body", err.Error()))
		return
	}
	word := strings.ToLower(strings.TrimSpace(req.Word))
	if err := validWord(word); err != nil {
		writeError(c, err)
		return
	}
	a.words.Add(word)
	if err := a.saveWords(); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"word": word})
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

// hideBanned returns messages as users should see them: any message with a
// banned word shows as "[removed]", the same as in the live feed. It changes
// only this copy; the store keeps the originals.
func (a *API) hideBanned(ms []chat.Message) []chat.Message {
	if a.words == nil {
		return ms
	}
	for i := range ms {
		if a.words.IsBanned(ms[i].Text) {
			ms[i].Text = "[removed]"
		}
	}
	return ms
}
