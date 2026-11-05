package api

import (
	"chatserver/internal/chat"
	"chatserver/internal/pb"
	"chatserver/internal/pbconv"
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

type createRoomReq struct {
	Name       string `json:"name" binding:"required"`
	MaxHistory int    `json:"max_history"`
}

// POST /rooms (logged in)
func (a *API) createRoom(c *gin.Context) {
	var req createRoomReq
	msg := &pb.CreateRoomRequest{}
	fill := func() {
		req.Name = msg.GetName()
		req.MaxHistory = int(msg.GetMaxHistory())
	}
	if err := bindBody(c, &req, msg, fill); err != nil {
		writeError(c, err)
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
	respond(c, http.StatusCreated, toRoomDTO(room), pbconv.Room(room))
}

// GET /rooms (logged in)
func (a *API) listRooms(c *gin.Context) {
	rooms := a.store.ListRooms()
	dtos := make([]roomDTO, len(rooms))
	pbRooms := make([]*pb.Room, len(rooms))
	for i, room := range rooms {
		dtos[i] = toRoomDTO(room)
		pbRooms[i] = pbconv.Room(room)
	}
	respond(c, http.StatusOK, dtos, &pb.ListRoomsResponse{Rooms: pbRooms})
}

// POST /rooms/:id/members (logged in) - the current user joins the room.
func (a *API) joinRoom(c *gin.Context) {
	room, err := a.store.GetRoom(chat.RoomID(c.Param("id")))
	if err != nil {
		writeError(c, err)
		return
	}
	if err := room.Join(currentUser(c).ID); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type postMessageReq struct {
	Text string `json:"text" binding:"required"`
}

// POST /rooms/:id/messages (logged in)
func (a *API) postMessage(c *gin.Context) {
	var req postMessageReq
	msg := &pb.PostMessageRequest{}
	if err := bindBody(c, &req, msg, func() { req.Text = msg.GetText() }); err != nil {
		writeError(c, err)
		return
	}
	posted, err := a.store.PostMessage(chat.RoomID(c.Param("id")), currentUser(c).ID, req.Text)
	if err != nil {
		writeError(c, err)
		return
	}
	if a.publish != nil {
		a.publish(posted)
	}
	respond(c, http.StatusCreated, toMessageDTO(posted), pbconv.Message(posted))
}

// GET /rooms/:id/messages?limit=50 (logged in)
func (a *API) history(c *gin.Context) {
	room, err := a.store.GetRoom(chat.RoomID(c.Param("id")))
	if err != nil {
		writeError(c, err)
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", defaultLimit))
	if err != nil || limit < 1 || limit > maxLimit {
		writeError(c, invalid("limit", "must be a number from 1 to 500"))
		return
	}
	messages := room.LastN(limit)
	respond(c, http.StatusOK, toMessageDTOs(messages), &pb.HistoryResponse{Messages: pbconv.Messages(messages)})
}
