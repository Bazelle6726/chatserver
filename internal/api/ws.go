package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"

	"chatserver/internal/chat"
	"chatserver/internal/hub"
	"chatserver/internal/ratelimit"
)

// GET /ws - chat over one WebSocket: log in, join rooms, send and receive.
// It's made for game clients (see the Unity package in unity/), which want
// one connection that works on every platform.
//
// Every frame, both ways, is one JSON object with a "type". Every field has
// the same meaning in every frame, so a client can use one class for all of
// them (Unity's JsonUtility needs exactly that). Any frame a client sends may
// carry a "ref", which the server copies onto its reply or error.
//
// Client to server:
//
//	{"type":"auth","token":"..."}              must be the first frame, within 10s
//	{"type":"join","room_name":"match-8841"}   by name: created (public) if missing, joined if allowed
//	{"type":"join","room_id":"r-3","limit":20} by ID; limit = history to send back (default 50)
//	{"type":"send","room_id":"r-3","text":"gg"}
//	{"type":"leave","room_id":"r-3"}           stop receiving the room (you stay a member)
//	{"type":"ping"}
//
// Server to client:
//
//	{"type":"ready","user":{...}}                      logged in
//	{"type":"joined","ref":"..","room_id":"r-3","room":{...},"history":[...]}
//	{"type":"message","message":{...}}                 a new message in a joined room
//	{"type":"sent","ref":"..","message":{...}}         your message was stored; it also arrives as "message"
//	{"type":"left","room_id":"r-3","reason":".."}      you left, or were dropped for reading too slowly
//	{"type":"error","ref":"..","code":"forbidden","error":"..."}
//	{"type":"pong","ref":".."}
//
// Error codes: invalid, unauthenticated, forbidden, not_found, conflict,
// rate_limited, too_large, unavailable, internal. An error about one request
// leaves the connection open; a failed login closes it.
//
// The token travels in the first frame, not the URL or a header, because
// browsers (and so Unity WebGL builds) can't set headers on a WebSocket, and
// URLs end up in logs.

// WebSocket limits.
const (
	wsAuthTimeout  = 10 * time.Second // to send the auth frame
	wsWriteTimeout = 10 * time.Second // for one frame to go out
	wsPingEvery    = 30 * time.Second // keep-alive, and to notice dead connections
	wsMaxFrame     = 16 << 10         // bytes in one frame from a client
	wsMaxRooms     = 20               // rooms one connection can receive at once
	wsOutbox       = 256              // frames queued for a client before it's too slow
)

// wsFrame is every frame, in and out.
type wsFrame struct {
	Type     string       `json:"type"`
	Ref      string       `json:"ref,omitempty"`
	Token    string       `json:"token,omitempty"`
	RoomName string       `json:"room_name,omitempty"`
	RoomID   string       `json:"room_id,omitempty"`
	Text     string       `json:"text,omitempty"`
	Limit    int          `json:"limit,omitempty"`
	User     *userDTO     `json:"user,omitempty"`
	Message  *messageDTO  `json:"message,omitempty"`
	History  []messageDTO `json:"history,omitempty"`
	Error    string       `json:"error,omitempty"`
	Code     string       `json:"code,omitempty"`
	Reason   string       `json:"reason,omitempty"`
	Room     *roomDTO     `json:"room,omitempty"`
}

// wsConn is one client's connection: who they are and what they're watching.
type wsConn struct {
	a      *API
	conn   *websocket.Conn
	user   *chat.User
	out    chan wsFrame // frames for the writer goroutine
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	watches map[chat.RoomID]func() // room -> stop receiving it
	wg      sync.WaitGroup         // the forwarder goroutines
}

func (a *API) websocket(c *gin.Context) {
	// Any Origin is accepted. The usual check stops other websites from
	// using a visitor's cookies to open a connection as them, but this
	// endpoint ignores cookies: it logs in only with the token in the first
	// frame, which another site doesn't have.
	conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return // Accept has already answered with an error
	}
	defer conn.CloseNow()
	conn.SetReadLimit(wsMaxFrame)

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	user, ref, err := a.wsAuth(ctx, conn)
	if err != nil {
		conn.Close(websocket.StatusPolicyViolation, "login failed: "+err.Error())
		return
	}
	c.Set(userKey, user) // so RequestLog names the user

	wc := &wsConn{a: a, conn: conn, user: user, out: make(chan wsFrame, wsOutbox), ctx: ctx, cancel: cancel, watches: map[chat.RoomID]func(){}}
	writerDone := make(chan struct{})
	go func() {
		wc.writer()
		close(writerDone)
	}()
	go wc.keepAlive()
	wc.send(wsFrame{Type: "ready", Ref: ref, User: ptr(toUserDTO(user))})

	wc.reader() // until the client goes, the server shuts down, or something fails

	// Tidy up in order: stop watching every room (ending the forwarders),
	// then let the writer finish what's queued.
	cancel()
	wc.mu.Lock()
	for _, stop := range wc.watches {
		stop()
	}
	wc.mu.Unlock()
	wc.wg.Wait()
	close(wc.out)
	<-writerDone
	conn.Close(websocket.StatusNormalClosure, "")
}

// wsAuth reads the first frame, which must log in.
func (a *API) wsAuth(ctx context.Context, conn *websocket.Conn) (*chat.User, string, error) {
	authCtx, cancel := context.WithTimeout(ctx, wsAuthTimeout)
	defer cancel()
	var f wsFrame
	if err := readFrame(authCtx, conn, &f); err != nil {
		return nil, "", err
	}
	if f.Type != "auth" {
		return nil, "", errors.New(`the first frame must be {"type":"auth","token":"..."}`)
	}
	user, err := a.svc.Authenticate(ctx, f.Token)
	if err != nil {
		return nil, "", err
	}
	if !a.limits.PerUser.Allow(string(user.ID)) {
		return nil, "", ratelimit.ErrLimited
	}
	return user, f.Ref, nil
}

// errBadFrame is a frame that isn't a JSON text object: the client gets an
// error frame, and the connection stays open.
var errBadFrame = invalid("frame", "must be a JSON object, sent as text")

// readFrame reads the next frame into f. A connection error is returned as
// is; a frame that can't be decoded is errBadFrame.
func readFrame(ctx context.Context, conn *websocket.Conn, f *wsFrame) error {
	typ, data, err := conn.Read(ctx)
	if err != nil {
		return err
	}
	if typ != websocket.MessageText || json.Unmarshal(data, f) != nil {
		return errBadFrame
	}
	return nil
}

// reader handles the client's frames one at a time until it stops.
func (wc *wsConn) reader() {
	// Hang up when the server shuts down: the HTTP server can't close a
	// WebSocket by itself, since it no longer owns the connection.
	go func() {
		select {
		case <-wc.a.svc.LiveDone():
			wc.conn.Close(websocket.StatusGoingAway, "server shutting down")
		case <-wc.ctx.Done():
		}
	}()

	for {
		var f wsFrame
		if err := readFrame(wc.ctx, wc.conn, &f); err != nil {
			if errors.Is(err, errBadFrame) {
				wc.fail("", err)
				continue
			}
			return // the client went away, or the connection failed
		}
		if !wc.a.limits.PerUser.Allow(string(wc.user.ID)) {
			wc.fail(f.Ref, ratelimit.ErrLimited)
			continue
		}
		wc.handle(f)
	}
}

func (wc *wsConn) handle(f wsFrame) {
	ctx := wc.ctx
	switch f.Type {
	case "join":
		wc.join(ctx, f)
	case "send":
		m, err := wc.a.svc.PostMessage(ctx, wc.user.ID, chat.RoomID(f.RoomID), f.Text)
		if err != nil {
			wc.fail(f.Ref, err)
			return
		}
		wc.send(wsFrame{Type: "sent", Ref: f.Ref, Message: ptr(toMessageDTO(m))})
	case "leave":
		if !wc.unwatch(chat.RoomID(f.RoomID)) {
			wc.fail(f.Ref, invalid("room_id", "not a room this connection has joined"))
			return
		}
		wc.send(wsFrame{Type: "left", Ref: f.Ref, RoomID: f.RoomID, Reason: "you left"})
	case "ping":
		wc.send(wsFrame{Type: "pong", Ref: f.Ref})
	case "auth":
		wc.fail(f.Ref, invalid("type", "already logged in"))
	default:
		wc.fail(f.Ref, invalid("type", `must be join, send, leave or ping`))
	}
}

// join enters a room (by name or ID), starts receiving it, and replies with
// the room and its recent history.
func (wc *wsConn) join(ctx context.Context, f wsFrame) {
	svc := wc.a.svc
	var room *chat.Room
	var err error
	switch {
	case f.RoomName != "" && f.RoomID != "":
		err = invalid("room_name", "give room_name or room_id, not both")
	case f.RoomName != "":
		room, err = svc.OpenRoom(ctx, wc.user.ID, f.RoomName)
	case f.RoomID != "":
		err = svc.JoinRoom(ctx, wc.user.ID, chat.RoomID(f.RoomID))
		if errors.Is(err, chat.ErrAlreadyExists) {
			err = nil // already a member is fine
		}
		if err == nil {
			room, err = svc.Room(ctx, chat.RoomID(f.RoomID))
		}
	default:
		err = invalid("room_name", "give room_name or room_id")
	}
	if err != nil {
		wc.fail(f.Ref, err)
		return
	}

	// Watch BEFORE reading the history, so nothing posted in between is
	// lost. The client drops duplicates by message ID.
	if err := wc.watch(ctx, room.ID); err != nil {
		wc.fail(f.Ref, err)
		return
	}
	history, err := svc.History(ctx, wc.user.ID, room.ID, f.Limit)
	if err != nil {
		wc.fail(f.Ref, err)
		return
	}
	wc.send(wsFrame{Type: "joined", Ref: f.Ref, RoomID: string(room.ID), Room: ptr(toRoomDTO(room)), History: toMessageDTOs(history)})
}

// watch starts receiving room's live messages, unless already receiving it.
func (wc *wsConn) watch(ctx context.Context, room chat.RoomID) error {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	if _, ok := wc.watches[room]; ok {
		return nil
	}
	if len(wc.watches) >= wsMaxRooms {
		return invalid("room", "this connection is already in 20 rooms: leave one first")
	}
	client, leave, err := wc.a.svc.Watch(ctx, wc.user.ID, room)
	if err != nil {
		return err
	}
	stop := make(chan struct{})
	var once sync.Once
	wc.watches[room] = func() {
		once.Do(func() {
			leave()
			close(stop)
		})
	}
	wc.wg.Add(1)
	go wc.forward(room, client, stop)
	return nil
}

// unwatch stops receiving room. It reports false if it wasn't being received.
func (wc *wsConn) unwatch(room chat.RoomID) bool {
	wc.mu.Lock()
	stop, ok := wc.watches[room]
	delete(wc.watches, room)
	wc.mu.Unlock()
	if ok {
		stop()
	}
	return ok
}

// forward copies one room's live messages to the client until it's told to
// stop, the connection ends, or the hub drops this client for being slow.
func (wc *wsConn) forward(room chat.RoomID, client *hub.Client, stop <-chan struct{}) {
	defer wc.wg.Done()
	for {
		select {
		case m, ok := <-client.Out():
			if !ok {
				// The hub closed our outbox: we fell behind, or it's shutting down.
				wc.mu.Lock()
				_, still := wc.watches[room]
				delete(wc.watches, room)
				wc.mu.Unlock()
				if still {
					wc.send(wsFrame{Type: "left", RoomID: string(room), Reason: "dropped for reading too slowly, or the server is shutting down: join again to catch up"})
				}
				return
			}
			wc.send(wsFrame{Type: "message", RoomID: string(room), Message: ptr(toMessageDTO(m))})
		case <-stop:
			return
		case <-wc.ctx.Done():
			return
		}
	}
}

// send queues a frame for the writer. If the client is so far behind that
// its queue is full, the connection is closed: it can reconnect and catch up
// from the history.
func (wc *wsConn) send(f wsFrame) {
	select {
	case wc.out <- f:
	case <-wc.ctx.Done():
	default:
		slog.Warn("websocket client too slow, closing", "user", wc.user.ID)
		wc.conn.Close(websocket.StatusPolicyViolation, "too slow")
		wc.cancel()
	}
}

// fail sends an error frame for the request with this ref.
func (wc *wsConn) fail(ref string, err error) {
	status := statusFor(err)
	msg := err.Error()
	if status == http.StatusInternalServerError {
		slog.Error("internal error", "route", "/ws", "err", err)
		msg = "internal error"
	}
	wc.send(wsFrame{Type: "error", Ref: ref, Code: errorCode(status), Error: msg})
}

// writer is the only goroutine that writes to the connection.
func (wc *wsConn) writer() {
	for f := range wc.out {
		data, err := json.Marshal(f)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), wsWriteTimeout)
		err = wc.conn.Write(ctx, websocket.MessageText, data)
		cancel()
		if err != nil {
			wc.cancel()
			// Keep draining, so senders never block on a dead connection.
		}
	}
}

// keepAlive pings the client now and then, and ends the connection if it
// stops answering.
func (wc *wsConn) keepAlive() {
	t := time.NewTicker(wsPingEvery)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			ctx, cancel := context.WithTimeout(wc.ctx, wsWriteTimeout)
			err := wc.conn.Ping(ctx)
			cancel()
			if err != nil {
				wc.cancel()
				return
			}
		case <-wc.ctx.Done():
			return
		}
	}
}

// errorCode names an HTTP status for error frames.
func errorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid"
	case http.StatusUnauthorized:
		return "unauthenticated"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusRequestEntityTooLarge:
		return "too_large"
	case http.StatusTooManyRequests:
		return "rate_limited"
	case http.StatusServiceUnavailable:
		return "unavailable"
	default:
		return "internal"
	}
}

func ptr[T any](v T) *T { return &v }
