// Package hub delivers new messages, live, to everyone connected to a room.
//
// The design: ONE goroutine (Run) owns the map of connected clients. Nobody
// else touches that map. Other goroutines ask the hub to do things by sending
// on its channels. Because only one goroutine uses the map, it needs no lock.
// "Don't communicate by sharing memory; share memory by communicating."
package hub

import (
	"context"
	"sync/atomic"

	"chatserver/internal/chat"
)

// Client is one live connection watching one room.
type Client struct {
	User chat.UserID
	Room chat.RoomID
	out  chan chat.Message // buffered; only the hub sends on it and closes it
}

// Out is where the client receives messages. It's receive-only (<-chan), so
// code outside the hub can't send on it or close it by mistake.
// It is closed when the hub removes the client.
func (c *Client) Out() <-chan chat.Message { return c.out }

// Hub tracks clients per room and fans messages out to them.
type Hub struct {
	register   chan *Client
	unregister chan *Client
	broadcast  chan chat.Message
	done       chan struct{} // closed when Run returns

	rooms   map[chat.RoomID]map[*Client]struct{} // ONLY touched by Run's goroutine
	buffer  int                                  // size of each client's outbox
	dropped atomic.Int64                         // safe to read from any goroutine
}

// NewHub makes a hub whose clients each get an outbox of buffer messages.
func NewHub(buffer int) *Hub {
	return &Hub{
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan chat.Message),
		done:       make(chan struct{}),
		rooms:      make(map[chat.RoomID]map[*Client]struct{}),
		buffer:     buffer,
	}
}

// Run is the hub's loop. Start it once, in its own goroutine: go h.Run(ctx).
// It returns when ctx is cancelled.
//
// TODO(39): loop forever with a select over four cases:
//   - a client arrives on h.register    -> h.add(c)
//   - a client arrives on h.unregister  -> h.remove(c)
//   - a message arrives on h.broadcast  -> h.fanOut(m)
//   - ctx.Done() is closed              -> h.closeAll(), then return
func (h *Hub) Run(ctx context.Context) {
	defer close(h.done) // tells Join/Leave/Publish the hub has stopped
}

// add puts c into its room's set, creating the set if needed.
//
// TODO(40): get h.rooms[c.Room]; if it's nil, make a new
// map[*Client]struct{} and store it; then set[c] = struct{}{}.
// (A map of empty structs is Go's usual "set": the values take no memory.)
func (h *Hub) add(c *Client) {
}

// remove takes c out of its room and closes its outbox.
//
// TODO(41):
//   - if c is NOT in h.rooms[c.Room], return straight away. remove can be
//     called twice for the same client (Leave after the hub already dropped it),
//     and closing a channel twice panics
//   - delete it from the set; if the set is now empty, delete the room too
//   - close(c.out): the hub is the only sender, so the hub closes it
func (h *Hub) remove(c *Client) {
}

// fanOut sends m to every client in m's room WITHOUT ever blocking.
//
// TODO(42): range over h.rooms[m.Room]. For each client:
//
//	select {
//	case c.out <- m:
//	    // delivered into its buffer
//	default:
//	    // buffer full: this client can't keep up. Drop it.
//	    h.remove(c)
//	    h.dropped.Add(1)
//	}
//
// A plain `c.out <- m` would block the whole hub on one slow client.
// (Deleting from a map while ranging over it is allowed in Go.)
func (h *Hub) fanOut(m chat.Message) {
}

// closeAll closes every client's outbox and forgets them all. Run calls it
// on shutdown so every reader sees its channel close and can finish.
//
// TODO(43): for every room, for every client: close(c.out).
// Then reset h.rooms to an empty map.
func (h *Hub) closeAll() {
}

// ---- Safe to call from any goroutine (already written) ----

// Join connects a new client to a room and returns it.
// If the hub has already stopped, the client's channel comes back closed.
func (h *Hub) Join(user chat.UserID, room chat.RoomID) *Client {
	c := &Client{User: user, Room: room, out: make(chan chat.Message, h.buffer)}
	select {
	case h.register <- c:
	case <-h.done:
		close(c.out) // the hub never saw it, so we're its only owner
	}
	return c
}

// Leave disconnects a client. Safe to call more than once.
func (h *Hub) Leave(c *Client) {
	select {
	case h.unregister <- c:
	case <-h.done:
	}
}

// Publish sends a message to everyone in its room. It never blocks forever:
// if the hub has stopped, the message is dropped.
func (h *Hub) Publish(m chat.Message) {
	select {
	case h.broadcast <- m:
	case <-h.done:
	}
}

// Dropped reports how many slow clients have been disconnected.
func (h *Hub) Dropped() int64 { return h.dropped.Load() }
