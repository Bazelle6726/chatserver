package api

import (
	"chatserver/internal/chat"
	"errors"
	"net/http"
)

// statusFor turns a domain error into an HTTP status code.
// This is the ONE place that knows how errors map to HTTP, so every
// endpoint answers the same way.
//
// Switch with errors.As / errors.Is (they also see through
// errors wrapped with %w):
func statusFor(err error) int {
	var ve *chat.ValidationError
	switch {
	case errors.As(err, &ve):
		return http.StatusBadRequest
	case errors.Is(err, chat.ErrUserNotFound), errors.Is(err, chat.ErrRoomNotFound):
		return http.StatusNotFound
	case errors.Is(err, chat.ErrAlreadyExists):
		return http.StatusConflict
	case errors.Is(err, chat.ErrNotMember):
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

// invalid builds a validation error for request problems the domain doesn't
// know about, like a bad ?limit= value.
func invalid(field, reason string) error {
	return &chat.ValidationError{Field: field, Reason: reason}
}
