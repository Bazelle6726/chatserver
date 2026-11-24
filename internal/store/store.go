package store

import (
	"context"

	"chatserver/internal/chat"
)

// Store is the interface the rest of the app depends on.
// MemoryStore implements it now; you could add a FileStore or Postgres store
// later without changing the HTTP or gRPC code. There's no "implements"
// keyword: any type with these methods satisfies Store.
//
// Every method takes a context.Context first. The memory store only checks
// it hasn't been cancelled, but a database store would pass it on to every
// query, so a request that times out or whose client goes away stops its
// database work too.
type Store interface {
	// Users. passwordHash is a bcrypt hash (see package auth), never a password.
	CreateUser(ctx context.Context, name string, passwordHash []byte) (*chat.User, error)
	GetUser(ctx context.Context, id chat.UserID) (*chat.User, error)
	UserByName(ctx context.Context, name string) (*chat.User, error)
	PasswordHash(ctx context.Context, id chat.UserID) ([]byte, error)

	CreateRoom(ctx context.Context, name string, maxHistory int, private bool) (*chat.Room, error)
	GetRoom(ctx context.Context, id chat.RoomID) (*chat.Room, error)
	RoomByName(ctx context.Context, name string) (*chat.Room, error)
	ListRooms(ctx context.Context) ([]*chat.Room, error)

	PostMessage(ctx context.Context, room chat.RoomID, author chat.UserID, text string) (chat.Message, error)
}

// Compile-time check that *MemoryStore satisfies Store.
// If you forget a method, the build fails here with a clear message.
var _ Store = (*MemoryStore)(nil)
