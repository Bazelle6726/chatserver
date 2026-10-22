package store

import "chatserver/internal/chat"

// Store is the interface the rest of the app depends on.
// MemoryStore implements it now; you could add a FileStore or Postgres store
// later without changing the HTTP or gRPC code. There's no "implements"
// keyword: any type with these methods satisfies Store.
type Store interface {
	CreateUser(name string) (*chat.User, error)
	GetUser(id chat.UserID) (*chat.User, error)

	CreateRoom(name string, maxHistory int) (*chat.Room, error)
	GetRoom(id chat.RoomID) (*chat.Room, error)
	ListRooms() []*chat.Room

	PostMessage(room chat.RoomID, author chat.UserID, text string) (chat.Message, error)
}

// Compile-time check that *MemoryStore satisfies Store.
// If you forget a method, the build fails here with a clear message.
var _ Store = (*MemoryStore)(nil)
