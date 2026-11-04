package store

import (
	"chatserver/internal/chat"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

// MemoryStore keeps everything in maps. mu protects both maps.
type MemoryStore struct {
	mu         sync.RWMutex
	users      map[chat.UserID]*chat.User
	rooms      map[chat.RoomID]*chat.Room
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
		nextUserID: NewIDGenerator("u"),
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
//   - validated with chat.ValidateName and returns the error as-is
//   - returns chat.ErrAlreadyExists if another user already has this name
//     (wrap it with context: fmt.Errorf("user %q: %w", name, chat.ErrAlreadyExists))
//   - otherwise the user is created with an ID from nextUserID, stores it, returns it
func (s *MemoryStore) CreateUser(name string) (*chat.User, error) {
	s.mu.Lock() // held for the duplicate check AND the insert
	defer s.mu.Unlock()
	if err := chat.ValidateName(name); err != nil {
		return nil, err
	}
	for _, user := range s.users {
		if user.Name == name {
			return nil, chat.ErrAlreadyExists
		}
	}
	user := &chat.User{
		ID:   chat.UserID(s.nextUserID()),
		Name: name,
		Role: chat.RoleMember,
	}
	s.users[user.ID] = user
	return user, nil
}

// Returns the user, or an error wrapping chat.ErrUserNotFound.
func (s *MemoryStore) GetUser(id chat.UserID) (*chat.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if user, exists := s.users[id]; exists {
		return user, nil
	}
	return nil, fmt.Errorf("user %q: %w", id, chat.ErrUserNotFound)
}

// Same rules as CreateUser, but for rooms. Use chat.NewRoom.
func (s *MemoryStore) CreateRoom(name string, maxHistory int) (*chat.Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := chat.ValidateName(name); err != nil {
		return nil, err
	}
	for _, room := range s.rooms {
		if room.Name == name {
			return nil, fmt.Errorf("room %q: %w", name, chat.ErrAlreadyExists)
		}
	}
	room := chat.NewRoom(chat.RoomID(s.nextRoomID()), name, maxHistory)
	s.rooms[room.ID] = room
	return room, nil
}

// Returns the room, or an error wrapping chat.ErrRoomNotFound.
func (s *MemoryStore) GetRoom(id chat.RoomID) (*chat.Room, error) {
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
func (s *MemoryStore) ListRooms() []*chat.Room {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rooms := make([]*chat.Room, 0, len(s.rooms))
	for _, room := range s.rooms {
		rooms = append(rooms, room)
	}
	slices.SortFunc(rooms, func(a, b *chat.Room) int {
		return strings.Compare(a.Name, b.Name)
	})
	return rooms
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
func (s *MemoryStore) PostMessage(roomID chat.RoomID, author chat.UserID, text string) (chat.Message, error) {
	room, err := s.GetRoom(roomID)
	if err != nil {
		return chat.Message{}, err
	}
	user, err := s.GetUser(author)
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
