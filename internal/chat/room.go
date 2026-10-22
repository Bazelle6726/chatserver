package chat

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
// Initialised BOTH maps with make() - writing to a nil map panics
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
// TODO(12):
//   - return ErrNotMember if msg.Author isn't in the room
//   - if the history is already full (len == maxHistory), drop the OLDEST
//     message first to make room
//   - then append the new message
//
// Careful: r.history = r.history[1:] looks right, but each reslice loses a slot
// at the front, so append keeps allocating new arrays (how append grows past capacity).
// Shift the kept messages to the front with copy() and shorten the slice instead,
// so the room reuses one backing array forever. A test checks the capacity.
func (r *Room) AddMessage(msg Message) error {
	return nil
}

// LastN returns up to n of the newest messages, oldest first.
//
// TODO(13): return a NEW slice (use make + copy), not a sub-slice of r.history.
// Otherwise the caller could modify the room's history through it, and it keeps
// the whole backing array alive (slices share their underlying array).
// If n <= 0 return an empty (non-nil) slice.
func (r *Room) LastN(n int) []Message {
	return nil
}

// SettingsCopy returns a copy of the settings so callers can't modify the room's map.
//
// TODO(14): use maps.Clone (a shallow copy).
func (r *Room) SettingsCopy() map[string]string {
	return r.Settings
}
