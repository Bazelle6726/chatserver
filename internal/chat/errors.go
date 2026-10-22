package chat

import (
	"errors"
	"fmt"
)

// Sentinel errors. Callers check them with errors.Is,
// which also works when they are wrapped with fmt.Errorf("...: %w", err).
var (
	ErrUserNotFound  = errors.New("user not found")
	ErrRoomNotFound  = errors.New("room not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrNotMember     = errors.New("user is not a member of the room")
)

// ValidationError is a custom error type. Callers check it with errors.As.
// In stage 2 it becomes an HTTP 400 response.
type ValidationError struct {
	Field  string
	Reason string
}

// Error makes *ValidationError satisfy the built-in error interface.
// Note the pointer receiver: it's *ValidationError that implements error.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid %s: %s", e.Field, e.Reason)
}
