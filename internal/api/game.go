package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// The game API is how a game's own backend (never the game client: the key
// must stay on your servers) signs its players in to chat and puts them in
// private match rooms. Every request carries the game key:
//
//	X-Game-Key: <CHAT_GAME_KEY>
//
// The flow:
//  1. A player logs in to your game. Your backend calls POST /game/sessions
//     with their player ID and display name, and passes the returned token
//     to the game client.
//  2. A match starts. Your backend calls POST /game/rooms with the match
//     name and its players.
//  3. The game client connects to /ws with the token and joins the match
//     room by name.
const gameKeyHeader = "X-Game-Key"

// WithGame turns on the game API with this key ("" leaves it off: its
// routes answer 404).
func (a *API) WithGame(key string) *API {
	a.gameKey = key
	return a
}

type gameSessionReq struct {
	PlayerID string `json:"player_id" binding:"required"`
	Name     string `json:"name"` // the player's display name in the game
}

// POST /game/sessions  {"player_id": "steam:76561198", "name": "Ace"}
// Signs the player in and returns {"user": ..., "token": ...}. The token is
// for the game client; no cookie is set, since the caller is a server.
func (a *API) gameSession(c *gin.Context) {
	var req gameSessionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, bodyError(err))
		return
	}
	user, token, err := a.svc.GameSession(c.Request.Context(), req.PlayerID, req.Name)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, authDTO{User: toUserDTO(user), Token: token})
}

type gameRoomReq struct {
	Name       string   `json:"name" binding:"required"`
	MaxHistory int      `json:"max_history"` // used only if the room is created; 0 = the default
	Players    []string `json:"players"`     // game player IDs, each signed in through /game/sessions
}

// POST /game/rooms  {"name": "match-8841", "players": ["steam:1", "steam:2"]}
// Finds or creates the private room and adds the players. Calling it again
// with more players adds them too.
func (a *API) gameRoom(c *gin.Context) {
	var req gameRoomReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, bodyError(err))
		return
	}
	room, err := a.svc.GameMatchRoom(c.Request.Context(), req.Name, req.MaxHistory, req.Players)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, toRoomDTO(room))
}
