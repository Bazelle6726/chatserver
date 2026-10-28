package api

import (
	"net/http"

	"chatserver/internal/chat"
)

// statusFor turns a domain error into an HTTP status code.
// This is the ONE place that knows how errors map to HTTP, so every
// endpoint answers the same way.
//
// TODO(23): use a switch with errors.As / errors.Is (they also see through
// errors wrapped with %w):
//
//	*chat.ValidationError          -> 400 http.StatusBadRequest
//	chat.ErrUserNotFound,
//	chat.ErrRoomNotFound           -> 404 http.StatusNotFound
//	chat.ErrAlreadyExists          -> 409 http.StatusConflict
//	chat.ErrNotMember              -> 403 http.StatusForbidden
//	anything else                  -> 500 http.StatusInternalServerError
//
// You'll need to import "errors".
func statusFor(err error) int {
	return http.StatusInternalServerError
}

// invalid builds a validation error for request problems the domain doesn't
// know about, like a bad ?limit= value.
func invalid(field, reason string) error {
	return &chat.ValidationError{Field: field, Reason: reason}
}
