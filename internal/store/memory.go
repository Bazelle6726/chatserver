package store

import (
	"time"

	"chatserver/internal/chat"
)

// MemoryStore keeps everything in maps. Not safe for concurrent use yet:
// stage 3 adds locks.
type MemoryStore struct {
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
// TODO(16): make both maps, create three ID generators with prefixes
// "u", "r" and "m", set now to time.Now, then loop over opts and call each one
// with the store.
func NewMemoryStore(opts ...Option) *MemoryStore {
	return &MemoryStore{}
}

// CreateUser validates the name and stores a new RoleMember user.
//
// TODO(17):
//   - validate with chat.ValidateName and return the error as-is
//   - return chat.ErrAlreadyExists if another user already has this name
//     (wrap it with context: fmt.Errorf("user %q: %w", name, chat.ErrAlreadyExists))
//   - otherwise create the user with an ID from nextUserID, store it, return it
func (s *MemoryStore) CreateUser(name string) (*chat.User, error) {
	return nil, nil
}

// TODO(18): return the user, or an error wrapping chat.ErrUserNotFound.
func (s *MemoryStore) GetUser(id chat.UserID) (*chat.User, error) {
	return nil, nil
}

// TODO(19): same rules as CreateUser, but for rooms. Use chat.NewRoom.
func (s *MemoryStore) CreateRoom(name string, maxHistory int) (*chat.Room, error) {
	return nil, nil
}

// TODO(20): return the room, or an error wrapping chat.ErrRoomNotFound.
func (s *MemoryStore) GetRoom(id chat.RoomID) (*chat.Room, error) {
	return nil, nil
}

// ListRooms returns all rooms sorted by name.
//
// TODO(21): map iteration order is random, so collect the
// rooms into a slice and sort it. Hint: slices.SortFunc with strings.Compare.
func (s *MemoryStore) ListRooms() []*chat.Room {
	return nil
}

// PostMessage validates and adds a text message to a room.
//
// TODO(22): in this order -
//   - look up the room (return its error)
//   - look up the author (return its error)
//   - validate the text with chat.ValidateMessage
//   - build a chat.Message with Room, Author, Text, Kind chat.KindText and
//     Meta{ID: next msg ID, CreatedAt: s.now()}
//   - add it with room.AddMessage and return the message
func (s *MemoryStore) PostMessage(roomID chat.RoomID, author chat.UserID, text string) (chat.Message, error) {
	return chat.Message{}, nil
}
