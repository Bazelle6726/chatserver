package chat

// Type definitions.
// UserID and RoomID are *distinct* types even though both are strings,
// so the compiler stops you passing a RoomID where a UserID is expected.
type UserID string
type RoomID string
type MessageID string
