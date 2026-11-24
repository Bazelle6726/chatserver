// Package service holds the chat server's use cases: create a room, post a
// message, read a history, watch a room. Both front doors, the HTTP API
// (package api) and the gRPC API (package grpcapi), call it, so every rule
// lives in exactly one place:
//
//   - defaults and limits (max_history, limit)
//   - who may do what (only members may read, watch or post)
//   - moderation (banned words show as "[removed]")
//   - live delivery (every posted message goes to the live feed)
//
// The front doors only translate: a request into a call here, and the result
// or error into a response.
//
//	HTTP (api) ──┐
//	             ├──► service ──► store, hub, moderation
//	gRPC (grpcapi)┘
package service

import (
	"context"
	"errors"

	"chatserver/internal/chat"
	"chatserver/internal/hub"
	"chatserver/internal/ratelimit"
	"chatserver/internal/store"
)

// Limits shared by both APIs.
const (
	DefaultMaxHistory = 100  // a room's history size when none is given
	MaxMaxHistory     = 1000 // the largest history a room may keep
	DefaultLimit      = 50   // messages a history read returns when none is asked for
	MaxLimit          = 500  // the most a history read may return
)

// ErrLiveOff means the server runs without live delivery (no hub), so a
// room can't be watched.
var ErrLiveOff = errors.New("live delivery is off")

// Service is the chat server's logic. Build it with New, then turn on the
// optional parts with WithHub and WithModeration.
type Service struct {
	store   store.Store
	hub     *hub.Hub             // nil = no live delivery: Watch returns ErrLiveOff
	publish func(m chat.Message) // nil = don't publish (e.g. in some tests)
	banned  func(string) bool    // nil = no moderation
	posts   *ratelimit.Limiter   // messages per user, however they're sent; nil = no limit
}

// New builds a service on top of a store.
func New(s store.Store) *Service {
	return &Service{store: s}
}

// WithHub turns on live delivery: publish is called with every new message
// (in main, the worker pool's Submit, which filters banned words and then
// hands it to the hub), and h is where watchers connect. Either may be nil.
func (s *Service) WithHub(h *hub.Hub, publish func(chat.Message)) *Service {
	s.hub = h
	s.publish = publish
	return s
}

// WithModeration makes History show messages with a banned word as
// "[removed]", the same as the live feed. In main, banned is the moderation
// list's IsBanned method.
func (s *Service) WithModeration(banned func(string) bool) *Service {
	s.banned = banned
	return s
}

// WithPostLimit limits how often each user may post. It applies however the
// message arrives: HTTP, a gRPC call, or one message among many in an
// Import or Converse stream, which per-request limits alone would miss.
func (s *Service) WithPostLimit(l *ratelimit.Limiter) *Service {
	s.posts = l
	return s
}

// invalid builds a validation error for a request field.
func invalid(field, reason string) error {
	return &chat.ValidationError{Field: field, Reason: reason}
}

// ---- Users ----

// CreateUser registers a new user.
func (s *Service) CreateUser(ctx context.Context, name string) (*chat.User, error) {
	return s.store.CreateUser(ctx, name)
}

// GetUser looks a user up by ID.
func (s *Service) GetUser(ctx context.Context, id chat.UserID) (*chat.User, error) {
	return s.store.GetUser(ctx, id)
}

// ---- Rooms ----

// CreateRoom creates a room keeping up to maxHistory messages; 0 means
// DefaultMaxHistory.
func (s *Service) CreateRoom(ctx context.Context, name string, maxHistory int) (*chat.Room, error) {
	if maxHistory == 0 {
		maxHistory = DefaultMaxHistory
	}
	if maxHistory < 1 || maxHistory > MaxMaxHistory {
		return nil, invalid("max_history", "must be 1-1000")
	}
	return s.store.CreateRoom(ctx, name, maxHistory)
}

// ListRooms returns every room, sorted by name.
func (s *Service) ListRooms(ctx context.Context) ([]*chat.Room, error) {
	return s.store.ListRooms(ctx)
}

// JoinRoom makes user a member of the room.
func (s *Service) JoinRoom(ctx context.Context, user chat.UserID, roomID chat.RoomID) error {
	room, err := s.store.GetRoom(ctx, roomID)
	if err != nil {
		return err
	}
	return room.Join(user)
}

// memberRoom looks a room up and checks user has joined it.
func (s *Service) memberRoom(ctx context.Context, user chat.UserID, roomID chat.RoomID) (*chat.Room, error) {
	room, err := s.store.GetRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if !room.IsMember(user) {
		return nil, chat.ErrNotMember
	}
	return room, nil
}

// ---- Messages ----

// PostMessage stores a message from user (who must be a member) and hands
// it to the live feed. A user posting faster than the post limit gets
// ratelimit.ErrLimited.
func (s *Service) PostMessage(ctx context.Context, user chat.UserID, roomID chat.RoomID, text string) (chat.Message, error) {
	if !s.posts.Allow(string(user)) {
		return chat.Message{}, ratelimit.ErrLimited
	}
	m, err := s.store.PostMessage(ctx, roomID, user, text)
	if err != nil {
		return chat.Message{}, err
	}
	if s.publish != nil {
		s.publish(m)
	}
	return m, nil
}

// History returns up to limit of the room's newest messages, oldest first,
// as users should see them (banned ones as "[removed]"). user must be a
// member. limit 0 means DefaultLimit.
func (s *Service) History(ctx context.Context, user chat.UserID, roomID chat.RoomID, limit int) ([]chat.Message, error) {
	if limit == 0 {
		limit = DefaultLimit
	}
	if limit < 1 || limit > MaxLimit {
		return nil, invalid("limit", "must be a number from 1 to 500")
	}
	room, err := s.memberRoom(ctx, user, roomID)
	if err != nil {
		return nil, err
	}
	return s.hideBanned(room.LastN(limit)), nil
}

// hideBanned replaces the text of banned messages in ms. ms must be a copy
// (room.LastN returns one), so the store keeps the originals.
func (s *Service) hideBanned(ms []chat.Message) []chat.Message {
	if s.banned == nil {
		return ms
	}
	for i := range ms {
		if s.banned(ms[i].Text) {
			ms[i].Text = "[removed]"
		}
	}
	return ms
}

// ---- Live feed ----

// Watch connects user (who must be a member) to the room's live feed. Read
// new messages from client.Out() until it closes, and call leave when done:
//
//	client, leave, err := svc.Watch(ctx, user, room)
//	if err != nil { ... }
//	defer leave()
func (s *Service) Watch(ctx context.Context, user chat.UserID, roomID chat.RoomID) (client *hub.Client, leave func(), err error) {
	if s.hub == nil {
		return nil, nil, ErrLiveOff
	}
	room, err := s.memberRoom(ctx, user, roomID)
	if err != nil {
		return nil, nil, err
	}
	client = s.hub.Join(user, room.ID)
	return client, func() { s.hub.Leave(client) }, nil
}

// JoinAndWatch is Watch for a user who may not have joined yet: it joins
// them first if needed. The gRPC Converse call uses it, since its first
// message is "join this room".
func (s *Service) JoinAndWatch(ctx context.Context, user chat.UserID, roomID chat.RoomID) (*hub.Client, func(), error) {
	if s.hub == nil {
		return nil, nil, ErrLiveOff
	}
	err := s.JoinRoom(ctx, user, roomID)
	if err != nil && !errors.Is(err, chat.ErrAlreadyExists) {
		return nil, nil, err
	}
	return s.Watch(ctx, user, roomID)
}
