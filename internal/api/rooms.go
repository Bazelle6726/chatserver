package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	defaultMaxHistory = 100
	maxMaxHistory     = 1000
	defaultLimit      = "50"
	maxLimit          = 500
)

// createRoomReq is the body for POST /rooms: {"name": "general", "max_history": 200}.
// max_history is optional; when it's missing it's 0 (the zero value).
type createRoomReq struct {
	Name       string `json:"name" binding:"required"`
	MaxHistory int    `json:"max_history"`
}

// POST /rooms  (logged in)
//
// TODO(31):
//   - bind the body like createUser does
//   - if MaxHistory is 0 use defaultMaxHistory; if it's below 0 or above
//     maxMaxHistory, writeError(c, invalid("max_history", "must be 1-1000"))
//   - a.store.CreateRoom(...), then respond 201 with toRoomDTO
func (a *API) createRoom(c *gin.Context) {
	c.Status(http.StatusNotImplemented)
}

// GET /rooms  (logged in)
//
// TODO(32): get a.store.ListRooms(), convert each room with toRoomDTO into a
// []roomDTO (make with a length, fill by index - see toMessageDTOs), and
// respond 200. Return an empty list, not null, when there are no rooms.
func (a *API) listRooms(c *gin.Context) {
	c.Status(http.StatusNotImplemented)
}

// POST /rooms/:id/members  (logged in) - the current user joins the room.
//
// TODO(33):
//   - look up the room from c.Param("id") (writeError if not found)
//   - room.Join(currentUser(c).ID) (writeError on error: ErrAlreadyExists -> 409)
//   - on success respond with no body: c.Status(http.StatusNoContent)
func (a *API) joinRoom(c *gin.Context) {
	c.Status(http.StatusNotImplemented)
}

// postMessageReq is the body for POST /rooms/:id/messages: {"text": "hi"}.
type postMessageReq struct {
	Text string `json:"text" binding:"required"`
}

// POST /rooms/:id/messages  (logged in)
//
// TODO(34):
//   - bind the body
//   - a.store.PostMessage(roomID, currentUser(c).ID, req.Text)
//     (the store already checks the room exists, membership and the text)
//   - respond 201 with toMessageDTO(msg), or writeError
func (a *API) postMessage(c *gin.Context) {
	c.Status(http.StatusNotImplemented)
}

// GET /rooms/:id/messages?limit=20  (logged in)
//
// TODO(35):
//   - look up the room (writeError if not found)
//   - read the limit with c.DefaultQuery("limit", defaultLimit) - it's a
//     STRING, so convert it with strconv.Atoi
//   - if Atoi fails or the number is < 1 or > maxLimit:
//     writeError(c, invalid("limit", "must be a number from 1 to 500"))
//   - respond 200 with toMessageDTOs(room.LastN(limit))
func (a *API) history(c *gin.Context) {
	c.Status(http.StatusNotImplemented)
}
