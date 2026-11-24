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
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"chatserver/internal/auth"
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

// Errors the front doors turn into status codes.
var (
	// ErrLiveOff means the server runs without live delivery (no hub), so a
	// room can't be watched.
	ErrLiveOff = errors.New("live delivery is off")

	// ErrBadCredentials is a failed login. It doesn't say whether the name or
	// the password was wrong, so it can't be used to find out who has an
	// account.
	ErrBadCredentials = errors.New("wrong name or password")

	// ErrPrivateRoom means someone tried to join a private (match) room that
	// the game didn't put them in.
	ErrPrivateRoom = errors.New("this room is private: only the game can add players to it")

	// ErrUnauthenticated means a request carried no session token, or one
	// that's unknown, logged out or expired.
	ErrUnauthenticated = errors.New("not logged in, or the session has expired")
)

// Service is the chat server's logic. Build it with New, then turn on the
// optional parts with WithHub and WithModeration.
type Service struct {
	store   store.Store
	hub     *hub.Hub             // nil = no live delivery: Watch returns ErrLiveOff
	publish func(m chat.Message) // nil = don't publish (e.g. in some tests)
	banned  func(string) bool    // nil = no moderation
	posts   *ratelimit.Limiter   // messages per user, however they're sent; nil = no limit

	sessions  *auth.Sessions
	cost      int       // bcrypt work factor
	dummyOnce sync.Once // makes dummyHash, once
	dummyHash []byte    // checked against when a login names an unknown user
}

// New builds a service on top of a store. Logins last auth.DefaultSessionTTL.
func New(s store.Store) *Service {
	return &Service{
		store:    s,
		sessions: auth.NewSessions(auth.DefaultSessionTTL, nil),
		cost:     bcrypt.DefaultCost,
	}
}

// WithPasswordCost sets bcrypt's work factor. Tests use bcrypt.MinCost,
// which is much faster (and much weaker); the default is bcrypt.DefaultCost.
func (s *Service) WithPasswordCost(cost int) *Service {
	s.cost = cost
	return s
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

// ---- Users and logins ----

// Register creates an account and logs it in, returning the new user and a
// session token.
func (s *Service) Register(ctx context.Context, name, password string) (*chat.User, string, error) {
	// Check the cheap things first: hashing takes tens of milliseconds, and
	// shouldn't be spent on a request that will fail anyway.
	if err := chat.ValidateName(name); err != nil {
		return nil, "", err
	}
	if err := auth.ValidatePassword(password); err != nil {
		return nil, "", err
	}
	hash, err := auth.HashPassword(password, s.cost)
	if err != nil {
		return nil, "", err
	}
	u, err := s.store.CreateUser(ctx, name, hash)
	if err != nil {
		return nil, "", err
	}
	token, err := s.sessions.Start(u.ID)
	if err != nil {
		return nil, "", err
	}
	return u, token, nil
}

// Login checks a name and password and starts a session, returning the user
// and a session token. Any mistake is ErrBadCredentials.
func (s *Service) Login(ctx context.Context, name, password string) (*chat.User, string, error) {
	u, err := s.store.UserByName(ctx, name)
	if errors.Is(err, chat.ErrUserNotFound) {
		// Check the password anyway, against a hash of nothing in particular,
		// so a login for an unknown name takes as long as one with a wrong
		// password. Otherwise the timing would reveal which names exist.
		auth.CheckPassword(s.dummy(), password)
		return nil, "", ErrBadCredentials
	}
	if err != nil {
		return nil, "", err
	}
	hash, err := s.store.PasswordHash(ctx, u.ID)
	if err != nil {
		return nil, "", err
	}
	if !auth.CheckPassword(hash, password) {
		return nil, "", ErrBadCredentials
	}
	token, err := s.sessions.Start(u.ID)
	if err != nil {
		return nil, "", err
	}
	return u, token, nil
}

// dummy returns a bcrypt hash at the service's cost, made once.
func (s *Service) dummy() []byte {
	s.dummyOnce.Do(func() {
		s.dummyHash, _ = auth.HashPassword("no password matches this", s.cost)
	})
	return s.dummyHash
}

// Logout ends the session with this token. An unknown token is fine.
func (s *Service) Logout(token string) {
	s.sessions.End(token)
}

// Authenticate returns the user a session token belongs to, or
// ErrUnauthenticated.
func (s *Service) Authenticate(ctx context.Context, token string) (*chat.User, error) {
	id, ok := s.sessions.Lookup(token)
	if !ok {
		return nil, ErrUnauthenticated
	}
	u, err := s.store.GetUser(ctx, id)
	if errors.Is(err, chat.ErrUserNotFound) {
		return nil, ErrUnauthenticated
	}
	return u, err
}

// SessionTTL is how long a login lasts (the HTTP API uses it for the
// cookie's lifetime).
func (s *Service) SessionTTL() time.Duration {
	return s.sessions.TTL()
}

// GetUser looks a user up by ID.
func (s *Service) GetUser(ctx context.Context, id chat.UserID) (*chat.User, error) {
	return s.store.GetUser(ctx, id)
}

// ---- Rooms ----

// CreateRoom creates a room keeping up to maxHistory messages; 0 means
// DefaultMaxHistory.
func (s *Service) CreateRoom(ctx context.Context, name string, maxHistory int) (*chat.Room, error) {
	return s.createRoom(ctx, name, maxHistory, false)
}

// createRoom checks maxHistory (0 = DefaultMaxHistory) and creates the room.
func (s *Service) createRoom(ctx context.Context, name string, maxHistory int, private bool) (*chat.Room, error) {
	if maxHistory == 0 {
		maxHistory = DefaultMaxHistory
	}
	if maxHistory < 1 || maxHistory > MaxMaxHistory {
		return nil, invalid("max_history", "must be 1-1000")
	}
	return s.store.CreateRoom(ctx, name, maxHistory, private)
}

// ListRooms returns the rooms user can see, sorted by name: every public
// room, and the private rooms they're in. Other people's match rooms stay
// hidden.
func (s *Service) ListRooms(ctx context.Context, user chat.UserID) ([]*chat.Room, error) {
	rooms, err := s.store.ListRooms(ctx)
	if err != nil {
		return nil, err
	}
	visible := make([]*chat.Room, 0, len(rooms))
	for _, r := range rooms {
		if !r.Private || r.IsMember(user) {
			visible = append(visible, r)
		}
	}
	return visible, nil
}

// JoinRoom makes user a member of the room. Private rooms can't be joined:
// only the game puts players in them (MatchRoom).
func (s *Service) JoinRoom(ctx context.Context, user chat.UserID, roomID chat.RoomID) error {
	room, err := s.store.GetRoom(ctx, roomID)
	if err != nil {
		return err
	}
	return joinPublic(room, user)
}

// joinPublic joins user to room, refusing private rooms they're not in.
func joinPublic(room *chat.Room, user chat.UserID) error {
	if room.Private && !room.IsMember(user) {
		return ErrPrivateRoom
	}
	return room.Join(user)
}

// OpenRoom is "take me to the room called name": it finds the room, creating
// a public one if there's none, and makes sure user is a member. A private
// room works only if the game already put user in it. A game client uses it
// to enter a lobby or its match room by name, without knowing the room's ID.
func (s *Service) OpenRoom(ctx context.Context, user chat.UserID, name string) (*chat.Room, error) {
	room, err := s.findOrCreateRoom(ctx, name, 0, false)
	if err != nil {
		return nil, err
	}
	if err := joinPublic(room, user); err != nil && !errors.Is(err, chat.ErrAlreadyExists) {
		return nil, err
	}
	return room, nil
}

// MatchRoom is for the game's backend: it finds the private room called
// name, creating it if there's none, and adds players to it. Only players
// the game adds can read or post in it. maxHistory applies only if the room
// is created (0 = DefaultMaxHistory).
func (s *Service) MatchRoom(ctx context.Context, name string, maxHistory int, players []chat.UserID) (*chat.Room, error) {
	room, err := s.findOrCreateRoom(ctx, name, maxHistory, true)
	if err != nil {
		return nil, err
	}
	if !room.Private {
		return nil, fmt.Errorf("room %q is public, so it can't be a match room: %w", name, chat.ErrAlreadyExists)
	}
	for _, p := range players {
		if _, err := s.store.GetUser(ctx, p); err != nil {
			return nil, err
		}
		if err := room.Join(p); err != nil && !errors.Is(err, chat.ErrAlreadyExists) {
			return nil, err
		}
	}
	return room, nil
}

// findOrCreateRoom returns the room called name, creating it if there's none.
func (s *Service) findOrCreateRoom(ctx context.Context, name string, maxHistory int, private bool) (*chat.Room, error) {
	room, err := s.store.RoomByName(ctx, name)
	if err == nil {
		return room, nil
	}
	if !errors.Is(err, chat.ErrRoomNotFound) {
		return nil, err
	}
	room, err = s.createRoom(ctx, name, maxHistory, private)
	if errors.Is(err, chat.ErrAlreadyExists) {
		// Someone created it between our lookup and our create: use theirs.
		return s.store.RoomByName(ctx, name)
	}
	return room, err
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
