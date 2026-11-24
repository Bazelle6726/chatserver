package store

import (
	"context"

	"chatserver/internal/chat"
	"chatserver/internal/textnorm"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

// MemoryStore keeps everything in maps. mu protects all four maps.
//
// userNames and roomNames are indexes keyed by each name's skeleton
// (textnorm.Skeleton), so a duplicate check is one map lookup instead of a
// scan of every user, and "Alice", "ALICE" and "аlice" (Cyrillic а) all
// count as the same name.
type MemoryStore struct {
	mu         sync.RWMutex
	users      map[chat.UserID]*chat.User
	rooms      map[chat.RoomID]*chat.Room
	userNames  map[string]chat.UserID
	passwords  map[chat.UserID][]byte // bcrypt hashes, kept apart from chat.User so they never reach an API response
	roomNames  map[string]chat.RoomID
	nextUserID func() string
	nextRoomID func() string
	nextMsgID  func() string
	now        func() time.Time // a function field, so tests can fake the clock
}

// Option configures a MemoryStore. NewMemoryStore takes any number of them
// (a variadic parameter) - the "functional options" pattern.
type Option func(*MemoryStore)

// WithClock replaces time.Now, so tests get predictable timestamps.
func WithClock(now func() time.Time) Option {
	return func(s *MemoryStore) { s.now = now }
}

// NewMemoryStore builds a store and applies the options in order.
//
// both maps are made with three ID generators with prefixes
// "u", "r" and "m", set now to time.Now, then looped over opts and calls each one
// with the store.
func NewMemoryStore(opts ...Option) *MemoryStore {
	s := &MemoryStore{
		users:      make(map[chat.UserID]*chat.User),
		rooms:      make(map[chat.RoomID]*chat.Room),
		userNames:  make(map[string]chat.UserID),
		passwords:  make(map[chat.UserID][]byte),
		roomNames:  make(map[string]chat.RoomID),
		nextUserID: NewRandomIDGenerator("u"),
		nextRoomID: NewIDGenerator("r"),
		nextMsgID:  NewIDGenerator("m"),
		now:        time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// CreateUser validates the name and stores a new RoleMember user.
//
//   - returns ctx.Err() if the context is already cancelled (every method
//     does this first)
//   - validates the name with chat.ValidateName and returns its error as-is
//   - returns an error wrapping chat.ErrAlreadyExists if another user already
//     has this name, or one that looks the same (see the userNames index)
//   - otherwise creates the user with an ID from nextUserID, stores it with
//     its password hash, and returns it
func (s *MemoryStore) CreateUser(ctx context.Context, name string, passwordHash []byte) (*chat.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock() // held for the duplicate check AND the insert
	defer s.mu.Unlock()
	if err := chat.ValidateName(name); err != nil {
		return nil, err
	}
	key := textnorm.Skeleton(name)
	if _, taken := s.userNames[key]; taken {
		return nil, fmt.Errorf("user %q: %w", name, chat.ErrAlreadyExists)
	}
	user := &chat.User{
		ID:   chat.UserID(s.nextUserID()),
		Name: name,
		Role: chat.RoleMember,
	}
	s.users[user.ID] = user
	s.userNames[key] = user.ID
	s.passwords[user.ID] = passwordHash
	return user, nil
}

// UserByName finds a user by name. Names are matched by their skeleton, the
// same way uniqueness is checked, so "Alice" finds "alice".
func (s *MemoryStore) UserByName(ctx context.Context, name string) (*chat.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if id, ok := s.userNames[textnorm.Skeleton(name)]; ok {
		return s.users[id], nil
	}
	return nil, fmt.Errorf("user %q: %w", name, chat.ErrUserNotFound)
}

// PasswordHash returns the user's bcrypt password hash.
func (s *MemoryStore) PasswordHash(ctx context.Context, id chat.UserID) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if hash, ok := s.passwords[id]; ok {
		return hash, nil
	}
	return nil, fmt.Errorf("user %q: %w", id, chat.ErrUserNotFound)
}

// Returns the user, or an error wrapping chat.ErrUserNotFound.
func (s *MemoryStore) GetUser(ctx context.Context, id chat.UserID) (*chat.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if user, exists := s.users[id]; exists {
		return user, nil
	}
	return nil, fmt.Errorf("user %q: %w", id, chat.ErrUserNotFound)
}

// Same rules as CreateUser, but for rooms. Use chat.NewRoom.
func (s *MemoryStore) CreateRoom(ctx context.Context, name string, maxHistory int) (*chat.Room, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := chat.ValidateName(name); err != nil {
		return nil, err
	}
	key := textnorm.Skeleton(name)
	if _, taken := s.roomNames[key]; taken {
		return nil, fmt.Errorf("room %q: %w", name, chat.ErrAlreadyExists)
	}
	room := chat.NewRoom(chat.RoomID(s.nextRoomID()), name, maxHistory)
	s.rooms[room.ID] = room
	s.roomNames[key] = room.ID
	return room, nil
}

// Returns the room, or an error wrapping chat.ErrRoomNotFound.
func (s *MemoryStore) GetRoom(ctx context.Context, id chat.RoomID) (*chat.Room, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if room, exists := s.rooms[id]; exists {
		return room, nil
	}
	return nil, fmt.Errorf("room %q: %w", id, chat.ErrRoomNotFound)
}

// ListRooms returns all rooms sorted by name.
//
// Map iteration order is random, so it collects the
// rooms into a slice and sorts it.
func (s *MemoryStore) ListRooms(ctx context.Context) ([]*chat.Room, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	rooms := make([]*chat.Room, 0, len(s.rooms))
	for _, room := range s.rooms {
		rooms = append(rooms, room)
	}
	slices.SortFunc(rooms, func(a, b *chat.Room) int {
		return strings.Compare(a.Name, b.Name)
	})
	return rooms, nil
}

// PostMessage validates and adds a text message to a room.
//
// In this order -
//   - looks up the room (returns its error)
//   - looks up the author (returns its error)
//   - validates the text with chat.ValidateMessage
//   - builds a chat.Message with Room, Author, Text, Kind chat.KindText and
//     Meta{ID: next msg ID, CreatedAt: s.now()}
//   - adds it with room.AddMessage and return the message
//
// No lock here: GetRoom and GetUser lock for themselves, and the room has its
// own lock for AddMessage. Locking here too would deadlock (locking twice).
func (s *MemoryStore) PostMessage(ctx context.Context, roomID chat.RoomID, author chat.UserID, text string) (chat.Message, error) {
	room, err := s.GetRoom(ctx, roomID)
	if err != nil {
		return chat.Message{}, err
	}
	user, err := s.GetUser(ctx, author)
	if err != nil {
		return chat.Message{}, err
	}

	if err := chat.ValidateMessage(text); err != nil {
		return chat.Message{}, err
	}
	msg := chat.Message{
		Room:   room.ID,
		Author: user.ID,
		Text:   text,
		Kind:   chat.KindText,
		Meta:   chat.Meta{ID: chat.MessageID(s.nextMsgID()), CreatedAt: s.now()},
	}
	if err := room.AddMessage(msg); err != nil {
		return chat.Message{}, err
	}
	return msg, nil
}
