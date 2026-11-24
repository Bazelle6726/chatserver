package grpcapi

import (
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"chatserver/internal/chat"
	"chatserver/internal/hub"
	"chatserver/internal/pb"
	"chatserver/internal/ratelimit"
	"chatserver/internal/service"
	"chatserver/internal/store"
)

// These tests run a real gRPC server and client, connected through bufconn:
// an in-memory network, so no port is opened. It's httptest for gRPC.

type env struct {
	t         *testing.T
	store     *store.MemoryStore
	svc       *service.Service
	ids       map[string]chat.UserID // session token -> user ID, for users made by e.user
	hub       *hub.Hub
	client    pb.ChatServiceClient
	published atomic.Int64 // how many messages reached publish
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{t: t, store: store.NewMemoryStore(), hub: hub.NewHub(16), ids: map[string]chat.UserID{}}
	hubCtx, stopHub := context.WithCancel(context.Background())
	go e.hub.Run(hubCtx)

	publish := func(m chat.Message) {
		e.published.Add(1)
		e.hub.Publish(m)
	}
	e.svc = service.New(e.store).WithHub(e.hub, publish).WithPasswordCost(bcrypt.MinCost)
	gs := grpc.NewServer(grpc.UnaryInterceptor(UnaryAuth(e.svc, ratelimit.Limits{})), grpc.StreamInterceptor(StreamAuth(e.svc, ratelimit.Limits{})))
	pb.RegisterChatServiceServer(gs, New(e.svc))
	lis := bufconn.Listen(1 << 20)
	go gs.Serve(lis)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	e.client = pb.NewChatServiceClient(conn)
	t.Cleanup(func() {
		conn.Close()
		gs.Stop()
		stopHub()
	})
	return e
}

// as returns a context for a call made with a session token ("" = not
// logged in), which fails after 2 seconds instead of hanging if something
// is stuck.
func (e *env) as(token string) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	e.t.Cleanup(cancel)
	if token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	}
	return ctx
}

// testPassword is every test user's password.
const testPassword = "correct-horse"

// user signs a user up and returns their session token. Setup goes straight
// through the service, so each test only exercises its own call.
func (e *env) user(name string) string {
	e.t.Helper()
	u, token, err := e.svc.Register(e.t.Context(), name, testPassword)
	if err != nil {
		e.t.Fatal(err)
	}
	e.ids[token] = u.ID
	return token
}

// id is the user ID of a token from e.user, as the API shows it.
func (e *env) id(token string) string {
	return string(e.ids[token])
}

// room creates a room with the users (tokens from e.user) as members.
func (e *env) room(name string, members ...string) string {
	e.t.Helper()
	r, err := e.store.CreateRoom(e.t.Context(), name, 100)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, m := range members {
		if err := r.Join(e.ids[m]); err != nil {
			e.t.Fatal(err)
		}
	}
	return string(r.ID)
}

func wantCode(t *testing.T, err error, want codes.Code, what string) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Fatalf("%s: got code %v (%v), want %v", what, got, err, want)
	}
}

func TestAuth(t *testing.T) {
	e := newEnv(t)

	// Signing up needs no session, and returns one.
	res, err := e.client.CreateUser(e.as(""), &pb.CreateUserRequest{Name: "alice", Password: testPassword})
	wantCode(t, err, codes.OK, "CreateUser without a session")
	if u := res.GetUser(); u.GetName() != "alice" || u.GetRole() != pb.Role_ROLE_MEMBER || res.GetToken() == "" {
		t.Errorf("CreateUser = %v, want alice, ROLE_MEMBER and a token", res)
	}
	token := res.GetToken()
	_, err = e.client.CreateUser(e.as(""), &pb.CreateUserRequest{Name: "x", Password: testPassword})
	wantCode(t, err, codes.InvalidArgument, "CreateUser with a bad name")
	_, err = e.client.CreateUser(e.as(""), &pb.CreateUserRequest{Name: "bobby", Password: "short"})
	wantCode(t, err, codes.InvalidArgument, "CreateUser with a short password")

	// Protected unary calls need a live session.
	_, err = e.client.ListRooms(e.as(""), &pb.ListRoomsRequest{})
	wantCode(t, err, codes.Unauthenticated, "ListRooms without a session")
	_, err = e.client.ListRooms(e.as("made-up-token"), &pb.ListRoomsRequest{})
	wantCode(t, err, codes.Unauthenticated, "ListRooms with a made-up token")
	_, err = e.client.ListRooms(e.as(token), &pb.ListRoomsRequest{})
	wantCode(t, err, codes.OK, "ListRooms with alice's token")

	// So do streaming calls. A stream's error arrives on the first Recv.
	w, err := e.client.Watch(e.as(""), &pb.WatchRequest{RoomId: "r-1"})
	if err == nil {
		_, err = w.Recv()
	}
	wantCode(t, err, codes.Unauthenticated, "Watch without a session")

	// Logging in gives a second session; a wrong password gives nothing.
	_, err = e.client.Login(e.as(""), &pb.LoginRequest{Name: "alice", Password: "wrong-password"})
	wantCode(t, err, codes.Unauthenticated, "Login with a wrong password")
	login, err := e.client.Login(e.as(""), &pb.LoginRequest{Name: "alice", Password: testPassword})
	wantCode(t, err, codes.OK, "Login")

	// Logging out ends only the session the call was made with.
	_, err = e.client.Logout(e.as(token), &pb.LogoutRequest{})
	wantCode(t, err, codes.OK, "Logout")
	_, err = e.client.ListRooms(e.as(token), &pb.ListRoomsRequest{})
	wantCode(t, err, codes.Unauthenticated, "ListRooms after Logout")
	_, err = e.client.ListRooms(e.as(login.GetToken()), &pb.ListRoomsRequest{})
	wantCode(t, err, codes.OK, "ListRooms with the other session")
}

func TestCreateAndListRooms(t *testing.T) {
	e := newEnv(t)
	alice := e.user("alice")

	list, err := e.client.ListRooms(e.as(alice), &pb.ListRoomsRequest{})
	wantCode(t, err, codes.OK, "ListRooms")
	if len(list.GetRooms()) != 0 {
		t.Fatalf("ListRooms on an empty server = %v", list.GetRooms())
	}

	r, err := e.client.CreateRoom(e.as(alice), &pb.CreateRoomRequest{Name: "general"})
	wantCode(t, err, codes.OK, "CreateRoom")
	if r.GetId() != "r-1" || r.GetName() != "general" {
		t.Errorf("CreateRoom = %v, want id r-1, name general", r)
	}
	_, err = e.client.CreateRoom(e.as(alice), &pb.CreateRoomRequest{Name: "general"})
	wantCode(t, err, codes.AlreadyExists, "duplicate room")
	_, err = e.client.CreateRoom(e.as(alice), &pb.CreateRoomRequest{Name: "big", MaxHistory: 5000})
	wantCode(t, err, codes.InvalidArgument, "max_history 5000")
	_, err = e.client.CreateRoom(e.as(alice), &pb.CreateRoomRequest{Name: "neg", MaxHistory: -1})
	wantCode(t, err, codes.InvalidArgument, "max_history -1")

	if _, err := e.client.CreateRoom(e.as(alice), &pb.CreateRoomRequest{Name: "art"}); err != nil {
		t.Fatal(err)
	}
	list, err = e.client.ListRooms(e.as(alice), &pb.ListRoomsRequest{})
	wantCode(t, err, codes.OK, "ListRooms")
	var names []string
	for _, r := range list.GetRooms() {
		names = append(names, r.GetName())
	}
	if len(names) != 2 || names[0] != "art" || names[1] != "general" {
		t.Errorf("ListRooms names = %v, want [art general]", names)
	}
}

func TestJoinPostAndHistory(t *testing.T) {
	e := newEnv(t)
	alice := e.user("alice")
	room := e.room("general")

	_, err := e.client.PostMessage(e.as(alice), &pb.PostMessageRequest{RoomId: room, Text: "too early"})
	wantCode(t, err, codes.PermissionDenied, "post before joining")
	_, err = e.client.JoinRoom(e.as(alice), &pb.JoinRoomRequest{RoomId: "r-404"})
	wantCode(t, err, codes.NotFound, "join a missing room")

	_, err = e.client.JoinRoom(e.as(alice), &pb.JoinRoomRequest{RoomId: room})
	wantCode(t, err, codes.OK, "join")
	_, err = e.client.JoinRoom(e.as(alice), &pb.JoinRoomRequest{RoomId: room})
	wantCode(t, err, codes.AlreadyExists, "join twice")

	for _, text := range []string{"one", "two", "three"} {
		m, err := e.client.PostMessage(e.as(alice), &pb.PostMessageRequest{RoomId: room, Text: text})
		wantCode(t, err, codes.OK, "post "+text)
		if m.GetText() != text || m.GetAuthorId() != e.id(alice) || m.GetRoomId() != room {
			t.Errorf("PostMessage returned %v", m)
		}
	}
	_, err = e.client.PostMessage(e.as(alice), &pb.PostMessageRequest{RoomId: room, Text: "   "})
	wantCode(t, err, codes.InvalidArgument, "blank message")
	if n := e.published.Load(); n != 3 {
		t.Errorf("published %d messages, want 3 (publish only after a successful post)", n)
	}

	h, err := e.client.History(e.as(alice), &pb.HistoryRequest{RoomId: room, Limit: 2})
	wantCode(t, err, codes.OK, "history")
	if got := h.GetMessages(); len(got) != 2 || got[0].GetText() != "two" || got[1].GetText() != "three" {
		t.Errorf("history limit 2 = %v, want [two three]", got)
	}
	h, err = e.client.History(e.as(alice), &pb.HistoryRequest{RoomId: room})
	wantCode(t, err, codes.OK, "history without a limit")
	if len(h.GetMessages()) != 3 {
		t.Errorf("history with limit 0 (default) returned %d messages, want 3", len(h.GetMessages()))
	}
	_, err = e.client.History(e.as(alice), &pb.HistoryRequest{RoomId: room, Limit: 501})
	wantCode(t, err, codes.InvalidArgument, "limit 501")
	_, err = e.client.History(e.as(alice), &pb.HistoryRequest{RoomId: "r-404"})
	wantCode(t, err, codes.NotFound, "history of a missing room")
}

// recv waits for one streamed message, or fails the test.
func recv(t *testing.T, next func() (*pb.Message, error), who string) *pb.Message {
	t.Helper()
	type result struct {
		m   *pb.Message
		err error
	}
	ch := make(chan result, 1)
	go func() { m, err := next(); ch <- result{m, err} }()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("%s: Recv failed: %v", who, r.err)
		}
		return r.m
	case <-time.After(time.Second):
		t.Fatalf("%s: no message within 1s", who)
	}
	return nil
}

func TestWatch(t *testing.T) {
	e := newEnv(t)
	alice := e.user("alice")
	room := e.room("general", alice)

	w, err := e.client.Watch(e.as(alice), &pb.WatchRequest{RoomId: room})
	if err != nil {
		t.Fatal(err)
	}
	// Header() waits until the server has joined the hub and sent its header,
	// so nothing posted after this can be missed.
	if _, err := w.Header(); err != nil {
		t.Fatalf("Header: %v (did Watch call stream.SendHeader?)", err)
	}

	for _, text := range []string{"first", "second"} {
		m, err := e.store.PostMessage(e.t.Context(), chat.RoomID(room), e.ids[alice], text)
		if err != nil {
			t.Fatal(err)
		}
		e.hub.Publish(m)
		if got := recv(t, w.Recv, "watcher"); got.GetText() != text || got.GetId() != string(m.ID) {
			t.Errorf("watcher got %v, want %q", got, text)
		}
	}

	missing, err := e.client.Watch(e.as(alice), &pb.WatchRequest{RoomId: "r-404"})
	if err == nil {
		_, err = missing.Recv()
	}
	wantCode(t, err, codes.NotFound, "Watch a missing room")
}

func TestImport(t *testing.T) {
	e := newEnv(t)
	alice := e.user("alice")
	room := e.room("general", alice)

	up, err := e.client.Import(e.as(alice))
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"a", "b", "c"} {
		if err := up.Send(&pb.PostMessageRequest{RoomId: room, Text: text}); err != nil {
			t.Fatalf("Send: %v", err)
		}
	}
	sum, err := up.CloseAndRecv()
	wantCode(t, err, codes.OK, "Import")
	if sum.GetPosted() != 3 {
		t.Errorf("posted = %d, want 3", sum.GetPosted())
	}
	r, _ := e.store.GetRoom(e.t.Context(), chat.RoomID(room))
	if n := len(r.LastN(10)); n != 3 {
		t.Errorf("room has %d messages after Import, want 3", n)
	}
	if n := e.published.Load(); n != 3 {
		t.Errorf("published %d, want 3", n)
	}

	// A bad message stops the import with that message's error.
	up, err = e.client.Import(e.as(alice))
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"ok", "   ", "never"} {
		if err := up.Send(&pb.PostMessageRequest{RoomId: room, Text: text}); err != nil {
			break // io.EOF: the server already ended the call; CloseAndRecv has the reason
		}
	}
	_, err = up.CloseAndRecv()
	wantCode(t, err, codes.InvalidArgument, "Import with a blank message")
}

func TestConverse(t *testing.T) {
	e := newEnv(t)
	alice := e.user("alice")
	bob := e.user("bob")
	room := e.room("general", alice) // bob isn't a member yet: Converse joins him

	open := func(user string) grpc.BidiStreamingClient[pb.ConverseRequest, pb.Message] {
		t.Helper()
		c, err := e.client.Converse(e.as(user))
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Send(&pb.ConverseRequest{Action: &pb.ConverseRequest_JoinRoomId{JoinRoomId: room}}); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Header(); err != nil {
			t.Fatalf("%s: Header: %v", user, err)
		}
		return c
	}
	a := open(alice)
	b := open(bob)

	say := func(c grpc.BidiStreamingClient[pb.ConverseRequest, pb.Message], text string) {
		t.Helper()
		if err := c.Send(&pb.ConverseRequest{Action: &pb.ConverseRequest_Text{Text: text}}); err != nil {
			t.Fatal(err)
		}
	}

	say(a, "hi bob")
	for _, who := range []struct {
		name string
		c    grpc.BidiStreamingClient[pb.ConverseRequest, pb.Message]
	}{{"alice", a}, {"bob", b}} {
		if m := recv(t, who.c.Recv, who.name); m.GetText() != "hi bob" || m.GetAuthorId() != e.id(alice) {
			t.Errorf("%s got %v, want alice's \"hi bob\"", who.name, m)
		}
	}
	say(b, "hi alice")
	if m := recv(t, a.Recv, "alice"); m.GetText() != "hi alice" || m.GetAuthorId() != e.id(bob) {
		t.Errorf("alice got %v, want bob's \"hi alice\"", m)
	}
	if m := recv(t, b.Recv, "bob"); m.GetText() != "hi alice" {
		t.Errorf("bob got %v", m)
	}

	// Closing our side ends the call cleanly: Recv returns io.EOF.
	if err := b.CloseSend(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		for {
			if _, err := b.Recv(); err != nil {
				done <- err
				return
			}
		}
	}()
	select {
	case err := <-done:
		if !errors.Is(err, io.EOF) {
			t.Errorf("after CloseSend, Recv returned %v, want io.EOF", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Converse didn't end after the client's CloseSend (is the reader passing io.EOF on?)")
	}

	// The first message must choose a room.
	c, err := e.client.Converse(e.as(alice))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Send(&pb.ConverseRequest{Action: &pb.ConverseRequest_Text{Text: "no room"}}); err != nil {
		t.Fatal(err)
	}
	_, err = c.Recv()
	wantCode(t, err, codes.InvalidArgument, "Converse without join_room_id first")
}
