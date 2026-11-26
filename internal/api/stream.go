package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GET /rooms/:id/stream  (logged in, member) - live messages as Server-Sent Events.
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
	client, leave, err := a.svc.Watch(c.Request.Context(), currentUser(c).ID, roomParam(c)) // only members may watch
	if err != nil {
		writeError(c, err)
		return
	}
	defer leave() // always tell the hub, however we exit

	w := c.Writer
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": connected\n\n") // an SSE comment, so the client sees the stream open
	w.Flush()

	for {
		select {
		case m, ok := <-client:
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
