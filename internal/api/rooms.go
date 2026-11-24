package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"chatserver/internal/chat"
	"chatserver/internal/pb"
	"chatserver/internal/pbconv"
)

// The handlers below only translate HTTP into service calls and back. The
// rules (defaults, limits, who may do what) are in package service.

type createRoomReq struct {
	Name       string `json:"name" binding:"required"`
	MaxHistory int    `json:"max_history"` // 0 = the default
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
	room, err := a.svc.CreateRoom(c.Request.Context(), req.Name, req.MaxHistory)
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusCreated, toRoomDTO(room), pbconv.Room(room))
}

// GET /rooms (logged in)
func (a *API) listRooms(c *gin.Context) {
	rooms, err := a.svc.ListRooms(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
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
	if err := a.svc.JoinRoom(c.Request.Context(), currentUser(c).ID, roomParam(c)); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type postMessageReq struct {
	Text string `json:"text" binding:"required"`
}

// POST /rooms/:id/messages (logged in, member)
func (a *API) postMessage(c *gin.Context) {
	var req postMessageReq
	msg := &pb.PostMessageRequest{}
	if err := bindBody(c, &req, msg, func() { req.Text = msg.GetText() }); err != nil {
		writeError(c, err)
		return
	}
	posted, err := a.svc.PostMessage(c.Request.Context(), currentUser(c).ID, roomParam(c), req.Text)
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusCreated, toMessageDTO(posted), pbconv.Message(posted))
}

// GET /rooms/:id/messages?limit=50 (logged in, member). No limit, or 0,
// means the default.
func (a *API) history(c *gin.Context) {
	limit := 0
	if s := c.Query("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			writeError(c, invalid("limit", "must be a number from 1 to 500"))
			return
		}
		limit = n
	}
	ms, err := a.svc.History(c.Request.Context(), currentUser(c).ID, roomParam(c), limit)
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusOK, toMessageDTOs(ms), &pb.HistoryResponse{Messages: pbconv.Messages(ms)})
}

// roomParam is the :id in the URL, as a room ID.
func roomParam(c *gin.Context) chat.RoomID {
	return chat.RoomID(c.Param("id"))
}
