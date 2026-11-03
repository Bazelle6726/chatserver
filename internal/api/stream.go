package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"chatserver/internal/chat"
)

// GET /rooms/:id/stream  (logged in) - live messages as Server-Sent Events.
//
// SSE is plain HTTP: the response simply never ends. Each new message is
// written as
//
//	event: message
//	data: {"id":"m-3","room":"r-1",...}
//
// followed by a blank line. Browsers can read it with `new EventSource(url)`,
// and curl.exe -N shows it live.
//
// This handler is one goroutine per watcher (Gin runs each request in its
// own goroutine). It waits on two channels at once with select:
// the client's outbox from the hub, and the request's context, which is
// cancelled when the watcher disconnects.
func (a *API) stream(c *gin.Context) {
	if a.hub == nil {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "live delivery is off"})
		return
	}
	room, err := a.store.GetRoom(chat.RoomID(c.Param("id")))
	if err != nil {
		writeError(c, err)
		return
	}

	client := a.hub.Join(currentUser(c).ID, room.ID)
	defer a.hub.Leave(client) // always tell the hub, however we exit

	w := c.Writer
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": connected\n\n") // an SSE comment, so the client sees the stream open
	w.Flush()

	for {
		select {
		case m, ok := <-client.Out():
			if !ok {
				return // the hub closed us: dropped as too slow, or shutting down
			}
			data, err := json.Marshal(toMessageDTO(m))
			if err != nil {
				return
			}
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
			w.Flush() // send it now, don't wait for a buffer to fill
		case <-c.Request.Context().Done():
			return // the watcher went away
		}
	}
}
