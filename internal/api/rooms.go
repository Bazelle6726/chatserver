package api

import (
	"chatserver/internal/chat"
	"net/http"
	"strconv"

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
	var req createRoomReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, invalid("body", err.Error()))
		return
	}
	if req.MaxHistory == 0 {
		req.MaxHistory = defaultMaxHistory
	}
	if req.MaxHistory < 0 || req.MaxHistory > maxMaxHistory {
		writeError(c, invalid("max_history", "must be 1-1000"))
		return
	}
	room, err := a.store.CreateRoom(req.Name, req.MaxHistory)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toRoomDTO(room))
}

// GET /rooms  (logged in)
func (a *API) listRooms(c *gin.Context) {
	rooms := a.store.ListRooms() //converts each room with toRoomDTO into a []roomDTO
	dtos := make([]roomDTO, len(rooms))
	for i, room := range rooms {
		dtos[i] = toRoomDTO(room)
	}
	c.JSON(http.StatusOK, dtos)
	//Return an empty list, not null, when there are no rooms.
}

// POST /rooms/:id/members  (logged in) - the current user joins the room.

func (a *API) joinRoom(c *gin.Context) {
	room, err := a.store.GetRoom(chat.RoomID(c.Param("id")))
	//looks uo the room from c.Param("id")

	if room == nil {
		writeError(c, err)
		return
	} //writesError if not found
	if err := room.Join(currentUser(c).ID); err != nil { //writes Error if already exists
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent) //responds with no body on success
}

// postMessageReq is the body for POST /rooms/:id/messages: {"text": "hi"}.
type postMessageReq struct {
	Text string `json:"text" binding:"required"`
}

// POST /rooms/:id/messages  (logged in)
func (a *API) postMessage(c *gin.Context) {
	var req postMessageReq
	if err := c.ShouldBindJSON(&req); err != nil { //binds the body
		writeError(c, invalid("body", err.Error()))
		return
	}
	room, err := a.store.PostMessage(chat.RoomID(c.Param("id")), currentUser(c).ID, req.Text)
	//store already checks the room exists, membership and the text
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toMessageDTO(room))
}

// GET /rooms/:id/messages?limit=20  (logged in)
func (a *API) history(c *gin.Context) {
	room, err := a.store.GetRoom(chat.RoomID(c.Param("id")))
	if room == nil {
		writeError(c, err)
		return
	} // look up the room (writeError if not found)
	limitStr := c.DefaultQuery("limit", defaultLimit) //read the limit
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > maxLimit {
		writeError(c, invalid("limit", "must be a number from 1 to 500"))
		return
	}
	messages := room.LastN(limit)
	dtos := toMessageDTOs(messages)
	c.JSON(http.StatusOK, dtos)
}
