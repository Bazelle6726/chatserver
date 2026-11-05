package api

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"

	"chatserver/internal/chat"
	"chatserver/internal/pb"
)

// --- TODOs 64-66: the three helpers, tested on their own -------------------

// testContext builds a Gin context for one request, without a router.
func testContext(method, contentType, accept string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, "/", bytes.NewReader(body))
	if contentType != "" {
		c.Request.Header.Set("Content-Type", contentType)
	}
	if accept != "" {
		c.Request.Header.Set("Accept", accept)
	}
	return c, rec
}

func TestWantsProto(t *testing.T) {
	tests := []struct {
		accept string
		want   bool
	}{
		{"", false},
		{"application/json", false},
		{"*/*", false},
		{"application/x-protobuf", true},
		{"application/protobuf", true},
		{"application/x-protobuf, application/json;q=0.5", true},
		{"text/html, application/protobuf", true},
	}
	for _, tt := range tests {
		c, _ := testContext("GET", "", tt.accept, nil)
		if got := wantsProto(c); got != tt.want {
			t.Errorf("Accept %q: wantsProto = %v, want %v", tt.accept, got, tt.want)
		}
	}
}

func TestRespond(t *testing.T) {
	dto := userDTO{ID: "u-1", Name: "alice", Role: "member"}
	msg := &pb.User{Id: "u-1", Name: "alice", Role: pb.Role_ROLE_MEMBER}

	// No Accept header: JSON, as before.
	c, rec := testContext("GET", "", "", nil)
	respond(c, http.StatusCreated, dto, msg)
	if rec.Code != http.StatusCreated {
		t.Errorf("JSON: status %d, want 201", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("JSON: Content-Type %q, want application/json", ct)
	}
	if got := decode[userDTO](t, rec); got != dto {
		t.Errorf("JSON body = %+v, want %+v", got, dto)
	}

	// Accept protobuf: the protobuf message, marshalled.
	c, rec = testContext("GET", "", mimeProto, nil)
	respond(c, http.StatusCreated, dto, msg)
	if rec.Code != http.StatusCreated {
		t.Errorf("protobuf: status %d, want 201", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != mimeProto {
		t.Errorf("protobuf: Content-Type %q, want %q", ct, mimeProto)
	}
	var got pb.User
	if err := proto.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("protobuf body doesn't decode: %v", err)
	}
	if !proto.Equal(&got, msg) {
		t.Errorf("protobuf body = %v, want %v", &got, msg)
	}
}

func TestBindBody(t *testing.T) {
	// JSON: straight into req; fill must NOT run.
	var req createUserReq
	msg := &pb.CreateUserRequest{}
	filled := false
	c, _ := testContext("POST", "application/json", "", []byte(`{"name":"alice"}`))
	if err := bindBody(c, &req, msg, func() { filled = true }); err != nil {
		t.Fatalf("JSON body: %v", err)
	}
	if req.Name != "alice" || filled {
		t.Errorf("JSON body: req = %+v, fill called = %v; want name alice and no fill", req, filled)
	}

	// Protobuf: into msg, then fill copies it into req.
	body, err := proto.Marshal(&pb.CreateUserRequest{Name: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	req = createUserReq{}
	msg = &pb.CreateUserRequest{}
	c, _ = testContext("POST", mimeProto, "", body)
	if err := bindBody(c, &req, msg, func() { req.Name = msg.GetName() }); err != nil {
		t.Fatalf("protobuf body: %v", err)
	}
	if req.Name != "bob" {
		t.Errorf("protobuf body: req.Name = %q, want bob (did you call fill?)", req.Name)
	}

	// Bad bodies, in either format, are validation errors on "body".
	for _, bad := range []struct {
		name, contentType string
		body              []byte
	}{
		{"broken JSON", "application/json", []byte(`{"name":`)},
		{"missing required name", "application/json", []byte(`{}`)},
		{"broken protobuf", mimeProto, []byte{0xff, 0xff, 0xff}},
	} {
		c, _ := testContext("POST", bad.contentType, "", bad.body)
		err := bindBody(c, &createUserReq{}, &pb.CreateUserRequest{}, func() {})
		var ve *chat.ValidationError
		if !errors.As(err, &ve) || ve.Field != "body" {
			t.Errorf("%s: got %v, want a *chat.ValidationError for field body", bad.name, err)
		}
	}
}

// --- TODOs 67-68: the handlers, through the router -------------------------

// doProto sends body as protobuf (nil = no body) and asks for protobuf back.
func (s *testServer) doProto(method, path string, body proto.Message, user string) *httptest.ResponseRecorder {
	s.t.Helper()
	var r *bytes.Reader
	if body != nil {
		b, err := proto.Marshal(body)
		if err != nil {
			s.t.Fatal(err)
		}
		r = bytes.NewReader(b)
	} else {
		r = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", mimeProto)
	}
	req.Header.Set("Accept", mimeProto)
	if user != "" {
		req.Header.Set("X-User-ID", user)
	}
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

// decodeProto checks the response is protobuf and decodes it into msg.
func decodeProto(t *testing.T, rec *httptest.ResponseRecorder, msg proto.Message, what string) {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != mimeProto {
		t.Fatalf("%s: Content-Type %q, want %q (body: %s)", what, ct, mimeProto, rec.Body.String())
	}
	if err := proto.Unmarshal(rec.Body.Bytes(), msg); err != nil {
		t.Fatalf("%s: response isn't valid protobuf: %v", what, err)
	}
}

func TestUsersInProtobuf(t *testing.T) {
	s := newServer(t)

	rec := s.doProto("POST", "/users", &pb.CreateUserRequest{Name: "alice"}, "")
	expectStatus(t, rec, http.StatusCreated, "POST /users (protobuf)")
	var u pb.User
	decodeProto(t, rec, &u, "POST /users")
	if u.GetId() != "u-1" || u.GetName() != "alice" || u.GetRole() != pb.Role_ROLE_MEMBER {
		t.Errorf("created %v, want u-1 alice ROLE_MEMBER", &u)
	}

	rec = s.doProto("GET", "/users/u-1", nil, "")
	expectStatus(t, rec, http.StatusOK, "GET /users/u-1 (protobuf)")
	var got pb.User
	decodeProto(t, rec, &got, "GET /users/u-1")
	if !proto.Equal(&got, &u) {
		t.Errorf("GET returned %v, want %v", &got, &u)
	}

	// JSON clients are unaffected.
	rec = s.do("GET", "/users/u-1", nil, "")
	expectStatus(t, rec, http.StatusOK, "GET /users/u-1 (JSON)")
	if d := decode[userDTO](t, rec); d.Name != "alice" {
		t.Errorf("JSON GET returned %+v", d)
	}

	// Errors stay JSON, whatever Accept says.
	rec = s.doProto("GET", "/users/u-404", nil, "")
	expectStatus(t, rec, http.StatusNotFound, "GET a missing user")
	if body := decode[map[string]any](t, rec); body["error"] == nil {
		t.Errorf("error body = %v, want JSON with an error field", body)
	}
}

func TestRoomsAndMessagesInProtobuf(t *testing.T) {
	s := newServer(t)
	alice := s.mustUser("alice") // JSON setup still works

	// An empty list is a valid protobuf message, and encodes to zero bytes.
	rec := s.doProto("GET", "/rooms", nil, alice)
	expectStatus(t, rec, http.StatusOK, "GET /rooms (empty)")
	var list pb.ListRoomsResponse
	decodeProto(t, rec, &list, "GET /rooms (empty)")
	if len(list.GetRooms()) != 0 {
		t.Errorf("empty server lists %v", list.GetRooms())
	}

	rec = s.doProto("POST", "/rooms", &pb.CreateRoomRequest{Name: "general", MaxHistory: 2}, alice)
	expectStatus(t, rec, http.StatusCreated, "POST /rooms (protobuf)")
	var room pb.Room
	decodeProto(t, rec, &room, "POST /rooms")
	if room.GetName() != "general" {
		t.Errorf("created room %v", &room)
	}
	s.mustRoom(alice, "art")

	rec = s.doProto("GET", "/rooms", nil, alice)
	expectStatus(t, rec, http.StatusOK, "GET /rooms")
	list = pb.ListRoomsResponse{}
	decodeProto(t, rec, &list, "GET /rooms")
	if rs := list.GetRooms(); len(rs) != 2 || rs[0].GetName() != "art" || rs[1].GetName() != "general" {
		t.Errorf("rooms = %v, want [art general]", rs)
	}

	path := "/rooms/" + room.GetId()
	expectStatus(t, s.do("POST", path+"/members", nil, alice), http.StatusNoContent, "join")

	for _, text := range []string{"one", "two", "three"} {
		rec = s.doProto("POST", path+"/messages", &pb.PostMessageRequest{Text: text}, alice)
		expectStatus(t, rec, http.StatusCreated, "post "+text)
		var m pb.Message
		decodeProto(t, rec, &m, "post "+text)
		if m.GetText() != text || m.GetAuthorId() != alice || m.GetKind() != pb.MessageKind_MESSAGE_KIND_TEXT {
			t.Errorf("posted %v", &m)
		}
	}

	// max_history 2 came through the protobuf body: only the last two remain.
	rec = s.doProto("GET", path+"/messages", nil, alice)
	expectStatus(t, rec, http.StatusOK, "history")
	var h pb.HistoryResponse
	decodeProto(t, rec, &h, "history")
	if ms := h.GetMessages(); len(ms) != 2 || ms[0].GetText() != "two" || ms[1].GetText() != "three" {
		t.Errorf("history = %v, want [two three] (was max_history copied in fill?)", ms)
	}

	// Mixed: a JSON body, a protobuf answer.
	req := httptest.NewRequest("POST", path+"/messages", strings.NewReader(`{"text":"mixed"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", mimeProto)
	req.Header.Set("X-User-ID", alice)
	rec = httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	expectStatus(t, rec, http.StatusCreated, "JSON in, protobuf out")
	var m pb.Message
	decodeProto(t, rec, &m, "JSON in, protobuf out")
	if m.GetText() != "mixed" {
		t.Errorf("got %v", &m)
	}

	// A protobuf body with blank text fails validation like JSON does.
	rec = s.doProto("POST", path+"/messages", &pb.PostMessageRequest{Text: "   "}, alice)
	expectStatus(t, rec, http.StatusBadRequest, "blank text in protobuf")
	if body := decode[map[string]any](t, rec); body["field"] != "text" {
		t.Errorf("error body = %v, want field text", body)
	}
}

