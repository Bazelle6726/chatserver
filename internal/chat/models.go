package chat

import "time"

// Meta is embedded by value into Message (struct embedding).
// Its fields are "promoted": you can write msg.ID instead of msg.Meta.ID.
type Meta struct {
	ID        MessageID
	CreatedAt time.Time
}

type User struct {
	ID   UserID
	Name string
	Role Role
}

type Message struct {
	Meta       // embedded by value: small, always present
	Room       RoomID
	Author     UserID
	AuthorName string // the author's name when posted, so clients needn't look it up
	Kind       MessageKind
	Text       string
}
