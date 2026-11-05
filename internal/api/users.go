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
// handler
func (a *API) createUser(c *gin.Context) { //JSON body is bound to a struct
	var req createUserReq
	msg := &pb.CreateUserRequest{}
	if err := bindBody(c, &req, msg, func() { req.Name = msg.Name }); err != nil {
		writeError(c, invalid("body", err.Error()))
		return
	}
	user, err := a.store.CreateUser(req.Name) //calls the store to create a user with the name from the request
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusCreated, toUserDTO(user), pbconv.User(user)) //responds in JSON and HTTP 201
}

// GET /users/:id

func (a *API) getUser(c *gin.Context) {
	id := c.Param("id") //ID is read and is converted to chat.UserID
	user, err := a.store.GetUser(chat.UserID(id))
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusOK, toUserDTO(user), pbconv.User(user))
}
