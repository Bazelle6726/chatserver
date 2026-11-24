package api

import (
	"chatserver/internal/chat"
	"chatserver/internal/pb"
	"chatserver/internal/pbconv"
	"net/http"

	"github.com/gin-gonic/gin"
)

// createUserReq is the JSON body for POST /users: {"name": "alice"}.
// binding:"required" makes ShouldBindJSON fail if name is missing or empty.
type createUserReq struct {
	Name string `json:"name" binding:"required"`
}

// POST /users
func (a *API) createUser(c *gin.Context) {
	var req createUserReq
	msg := &pb.CreateUserRequest{}
	if err := bindBody(c, &req, msg, func() { req.Name = msg.Name }); err != nil {
		writeError(c, err) // bindBody already says which field, or 413
		return
	}
	user, err := a.svc.CreateUser(c.Request.Context(), req.Name)
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusCreated, toUserDTO(user), pbconv.User(user))
}

// GET /users/:id
func (a *API) getUser(c *gin.Context) {
	user, err := a.svc.GetUser(c.Request.Context(), chat.UserID(c.Param("id")))
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusOK, toUserDTO(user), pbconv.User(user))
}
