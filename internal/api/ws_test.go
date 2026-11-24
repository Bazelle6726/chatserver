package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"chatserver/internal/chat"
	"chatserver/internal/hub"
	"chatserver/internal/moderation"
	"chatserver/internal/ratelimit"
	"chatserver/internal/service"
	"chatserver/internal/store"
)

// wsEnv is a real server on a local port, with live delivery, for
// WebSocket tests.
type wsEnv struct {
	t       *testing.T
	svc     *service.Service
	url     string
	stopHub context.CancelFunc
}

func newWSEnv(t *testing.T, limits ...ratelimit.Limits) *wsEnv {
	t.Helper()
	h := hub.NewHub(16)
	hubCtx, stopHub := context.WithCancel(context.Background())
	go h.Run(hubCtx)
	svc := newSvcOn(store.NewMemoryStore()).WithHub(h, h.Publish).WithModeration(moderation.New("scam").IsBanned)
	var l ratelimit.Limits
	if len(limits) > 0 {
		l = limits[0]
	}
	srv := httptest.NewServer(New(svc, l).Router())
	t.Cleanup(func() {
		stopHub()
		srv.Close()
	})
	return &wsEnv{t: t, svc: svc, url: "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws", stopHub: stopHub}
}

// user signs a user up and returns their ID and token.
func (e *wsEnv) user(name string) (chat.UserID, string) {
	e.t.Helper()
	u, token, err := e.svc.Register(e.t.Context(), name, testPassword)
	if err != nil {
		e.t.Fatal(err)
	}
	return u.ID, token
}

// wsClient is one test connection.
type wsClient struct {
	t    *testing.T
	conn *websocket.Conn
}

// dial connects without logging in.
func (e *wsEnv) dial() *wsClient {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, e.url, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { conn.CloseNow() })
	return &wsClient{t: e.t, conn: conn}
}

// login connects and logs in with token, and checks the "ready" reply.
func (e *wsEnv) login(token string) *wsClient {
	e.t.Helper()
	c := e.dial()
	c.send(wsFrame{Type: "auth", Token: token})
	if f := c.next(); f.Type != "ready" || f.User == nil {
		e.t.Fatalf("after auth got %+v, want ready with the user", f)
	}
	return c
}

func (c *wsClient) send(f wsFrame) {
	c.t.Helper()
	data, _ := json.Marshal(f)
	c.sendRaw(string(data))
}

func (c *wsClient) sendRaw(s string) {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.conn.Write(ctx, websocket.MessageText, []byte(s)); err != nil {
		c.t.Fatal(err)
	}
}

// next reads the next frame, failing the test after 2 seconds.
func (c *wsClient) next() wsFrame {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, data, err := c.conn.Read(ctx)
	if err != nil {
		c.t.Fatalf("reading a frame: %v", err)
	}
	var f wsFrame
	if err := json.Unmarshal(data, &f); err != nil {
		c.t.Fatalf("frame %s isn't JSON: %v", data, err)
	}
	return f
}

// nextOf skips frames until one of the given type arrives.
func (c *wsClient) nextOf(typ string) wsFrame {
	c.t.Helper()
	for range 10 {
		if f := c.next(); f.Type == typ {
			return f
		}
	}
	c.t.Fatalf("no %q frame in the next 10", typ)
	return wsFrame{}
}

// closed checks the server closes the connection with this status.
func (c *wsClient) closed(want websocket.StatusCode, what string) {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for {
		_, _, err := c.conn.Read(ctx)
		if err == nil {
			continue
		}
		if got := websocket.CloseStatus(err); got != want {
			c.t.Fatalf("%s: connection ended with %v (%v), want close status %v", what, got, err, want)
		}
		return
	}
}

func TestWSLoginRequired(t *testing.T) {
	e := newWSEnv(t)

	c := e.dial()
	c.send(wsFrame{Type: "join", RoomName: "lobby"})
	c.closed(wsCloseBadLogin, "a first frame that isn't auth")

	c = e.dial()
	c.send(wsFrame{Type: "auth", Token: "made-up-token"})
	c.closed(wsCloseBadLogin, "a made-up token")
}

func TestWSChat(t *testing.T) {
	e := newWSEnv(t)
	_, aliceToken := e.user("alice")
	bobID, bobToken := e.user("bob")
	alice, bob := e.login(aliceToken), e.login(bobToken)

	// Alice opens "lobby" by name: it's created and she's in it.
	alice.send(wsFrame{Type: "join", Ref: "j1", RoomName: "lobby"})
	joined := alice.nextOf("joined")
	if joined.Ref != "j1" || joined.Room == nil || joined.Room.Name != "lobby" || joined.RoomID == "" {
		t.Fatalf("joined = %+v, want lobby with ref j1", joined)
	}
	lobby := joined.RoomID

	// Bob joins by ID.
	bob.send(wsFrame{Type: "join", RoomID: lobby})
	bob.nextOf("joined")

	// Alice posts: she gets an ack with her ref, and both get the message,
	// with her name on it.
	alice.send(wsFrame{Type: "send", Ref: "s1", RoomID: lobby, Text: "hi bob"})
	sent := alice.nextOf("sent")
	if sent.Ref != "s1" || sent.Message == nil || sent.Message.Text != "hi bob" {
		t.Fatalf("sent = %+v, want the stored message with ref s1", sent)
	}
	for _, who := range []*wsClient{alice, bob} {
		m := who.nextOf("message")
		if m.Message.ID != sent.Message.ID || m.Message.AuthorName != "alice" || m.RoomID != lobby {
			t.Errorf("live message = %+v, want alice's message in the lobby", m.Message)
		}
	}

	// A banned word arrives in the history as [removed].
	alice.send(wsFrame{Type: "send", RoomID: lobby, Text: "a total scam"})
	alice.nextOf("sent")

	// Joining again (here, Bob on a new connection) brings back the history.
	bob2 := e.login(bobToken)
	bob2.send(wsFrame{Type: "join", RoomName: "lobby", Limit: 10})
	j := bob2.nextOf("joined")
	if len(j.History) != 2 || j.History[0].Text != "hi bob" || j.History[1].Text != "[removed]" {
		t.Errorf("history = %+v, want hi bob then [removed]", j.History)
	}

	// Leaving stops the feed but keeps the membership.
	bob.send(wsFrame{Type: "leave", Ref: "l1", RoomID: lobby})
	if f := bob.nextOf("left"); f.Ref != "l1" {
		t.Errorf("left = %+v, want ref l1", f)
	}
	room, _ := e.svc.Room(t.Context(), chat.RoomID(lobby))
	if !room.IsMember(bobID) {
		t.Error("leave removed bob from the room; it should only stop the feed")
	}

	// Errors come back as frames, and the connection stays open.
	alice.send(wsFrame{Type: "send", Ref: "s2", RoomID: "r-404", Text: "anyone?"})
	if f := alice.nextOf("error"); f.Ref != "s2" || f.Code != "not_found" {
		t.Errorf("send to a missing room = %+v, want not_found with ref s2", f)
	}
	alice.send(wsFrame{Type: "ping", Ref: "p1"})
	if f := alice.nextOf("pong"); f.Ref != "p1" {
		t.Errorf("pong = %+v, want ref p1", f)
	}
}

func TestWSPrivateRoom(t *testing.T) {
	e := newWSEnv(t)
	aliceID, aliceToken := e.user("alice")
	_, eveToken := e.user("eve")
	if _, err := e.svc.MatchRoom(t.Context(), "match-1", 0, []chat.UserID{aliceID}); err != nil {
		t.Fatal(err)
	}

	alice := e.login(aliceToken)
	alice.send(wsFrame{Type: "join", RoomName: "match-1"})
	if j := alice.nextOf("joined"); j.Room == nil || !j.Room.Private {
		t.Errorf("alice's join = %+v, want the private match room", j)
	}

	eve := e.login(eveToken)
	eve.send(wsFrame{Type: "join", Ref: "j", RoomName: "match-1"})
	if f := eve.nextOf("error"); f.Code != "forbidden" {
		t.Errorf("eve's join = %+v, want forbidden", f)
	}
}

func TestWSBadFrames(t *testing.T) {
	e := newWSEnv(t)
	_, token := e.user("alice")
	c := e.login(token)

	c.sendRaw("this is not json")
	if f := c.next(); f.Type != "error" || f.Code != "invalid" {
		t.Errorf("non-JSON frame = %+v, want an invalid error", f)
	}
	c.send(wsFrame{Type: "dance", Ref: "d"})
	if f := c.next(); f.Type != "error" || f.Ref != "d" {
		t.Errorf("unknown type = %+v, want an error with ref d", f)
	}
	c.send(wsFrame{Type: "join"})
	if f := c.next(); f.Type != "error" || f.Code != "invalid" {
		t.Errorf("join with no room = %+v, want an invalid error", f)
	}
	// Still connected.
	c.send(wsFrame{Type: "ping"})
	c.nextOf("pong")
}

// When the server shuts down, WebSockets are closed with "going away".
func TestWSShutdown(t *testing.T) {
	e := newWSEnv(t)
	_, token := e.user("alice")
	c := e.login(token)
	c.send(wsFrame{Type: "join", RoomName: "lobby"})
	c.nextOf("joined")

	e.stopHub()
	c.closed(websocket.StatusGoingAway, "server shutting down")
}

// A rate-limited login is worth retrying later (1013), unlike a refused
// token (4401): clients must be able to tell them apart.
func TestWSLoginRateLimited(t *testing.T) {
	e := newWSEnv(t, ratelimit.Limits{PerUser: ratelimit.New(0.001, 1)}) // one request, then nothing for ages
	_, token := e.user("alice")
	e.login(token) // uses the only request
	c := e.dial()
	c.send(wsFrame{Type: "auth", Token: token})
	c.closed(websocket.StatusTryAgainLater, "a rate-limited login")
}

// Frames don't escape < > & into 6-byte codes: it's not HTML, and it would make
// a frame of history up to 6 times bigger.
func TestWSFramesAreNotHTMLEscaped(t *testing.T) {
	e := newWSEnv(t)
	_, token := e.user("alice")
	c := e.login(token)
	c.send(wsFrame{Type: "join", RoomName: "lobby"})
	room := c.nextOf("joined").RoomID
	c.send(wsFrame{Type: "send", RoomID: room, Text: "a <b> & c"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), `"type":"sent"`) {
			if !strings.Contains(string(data), "a <b> & c") {
				t.Errorf("frame %s escapes the text, want it as typed", data)
			}
			return
		}
	}
}
