package chat

import "maps"

// Room holds members and a capped message history.
// No locks yet - stage 3 adds a sync.RWMutex when goroutines arrive.
type Room struct {
	ID         RoomID
	Name       string
	Settings   map[string]string
	members    map[UserID]struct{} // empty struct as a "set": uses no memory per value
	history    []Message
	maxHistory int
}

// NewRoom creates a room that keeps at most maxHistory messages.
//
// Initialized BOTH maps with make() - writing to a nil map panics
// (nil map rule). History has a capacity of maxHistory.
func NewRoom(id RoomID, name string, maxHistory int) *Room {
	return &Room{
		ID:         id,
		Name:       name,
		Settings:   make(map[string]string),
		members:    make(map[UserID]struct{}),
		history:    make([]Message, 0, maxHistory),
		maxHistory: maxHistory,
	}
}

// User added to members. ErrAlreadyExists returns if already there.
func (r *Room) Join(id UserID) error {
	if r.IsMember(id) {
		return ErrAlreadyExists
	}
	r.members[id] = struct{}{}
	return nil
}

// Returns ErrNotMember if they weren't a member.
func (r *Room) Leave(id UserID) error {
	if !r.IsMember(id) {
		return ErrNotMember
	}
	delete(r.members, id)
	return nil
}

// IsMember uses the "comma ok" map read.
func (r *Room) IsMember(id UserID) bool {
	_, ok := r.members[id]
	return ok
}

// MemberCount returns the number of members.
func (r *Room) MemberCount() int {
	return len(r.members)
}

// AddMessage appends a message to the history.
//
//   - ErrNotMember returns if msg.Author isn't in the room
//   - if the history is already full (len == maxHistory), the OLDEST message drops
//     first to make room
//   - then the new message appends to the end of the slice (newest last)
func (r *Room) AddMessage(msg Message) error {
	if !r.IsMember(msg.Author) {
		return ErrNotMember
	}
	if len(r.history) == r.maxHistory {
		copy(r.history, r.history[1:])
		r.history = r.history[:r.maxHistory-1]
	}
	r.history = append(r.history, msg)
	return nil
}

// LastN returns up to n of the newest messages, oldest first.
//
// A NEW slice is returned (use make + copy), not a sub-slice of r.history.
// Otherwise the caller could modify the room's history through it, and it keeps
// the whole backing array alive (slices share their underlying array).
// If n <= 0 return an empty (non-nil) slice.
func (r *Room) LastN(n int) []Message {
	if n <= 0 {
		return make([]Message, 0)
	}
	if n > len(r.history) {
		n = len(r.history)
	}
	result := make([]Message, n)
	copy(result, r.history[len(r.history)-n:])
	return result
}

// SettingsCopy returns a copy of the settings so callers can't modify the room's map.
//
// maps.Clone returns a shallow copy
func (r *Room) SettingsCopy() map[string]string {
	return maps.Clone(r.Settings)
}
