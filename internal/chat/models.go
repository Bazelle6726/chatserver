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
	Meta   // embedded by value: small, always present
	Room   RoomID
	Author UserID
	Kind   MessageKind
	Text   string
}
