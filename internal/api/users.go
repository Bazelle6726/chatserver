package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// createUserReq is the JSON body for POST /users: {"name": "alice"}.
// binding:"required" makes ShouldBindJSON fail if name is missing or empty.
type createUserReq struct {
	Name string `json:"name" binding:"required"`
}

// POST /users
//
// TODO(29): the three steps every handler follows.
//
// 1. Bind the JSON body into the request struct:
//
//	var req createUserReq
//	if err := c.ShouldBindJSON(&req); err != nil {
//		writeError(c, invalid("body", err.Error()))
//		return
//	}
//
// 2. Call the store: a.store.CreateUser(req.Name). On error, writeError(c, err) and return.
//
// 3. Respond: c.JSON(http.StatusCreated, toUserDTO(user))
func (a *API) createUser(c *gin.Context) {
	c.Status(http.StatusNotImplemented)
}

// GET /users/:id
//
// TODO(30): read the id with c.Param("id"), convert it to chat.UserID, call
// a.store.GetUser, and respond 200 with toUserDTO, or writeError on error.
// (You'll need to import "chatserver/internal/chat".)
func (a *API) getUser(c *gin.Context) {
	c.Status(http.StatusNotImplemented)
}
