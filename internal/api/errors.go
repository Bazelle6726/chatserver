package api

import (
	"context"
	"errors"
	"net/http"

	"chatserver/internal/chat"
	"chatserver/internal/ratelimit"
	"chatserver/internal/service"
)

// statusClientClosedRequest is for a request whose client went away before
// it finished (its context was cancelled). It's not in net/http: the code
// comes from nginx. Nobody will see the response, but the log will.
const statusClientClosedRequest = 499

// statusFor turns a domain error into an HTTP status code.
// This is the ONE place that knows how errors map to HTTP, so every
// endpoint answers the same way.
//
// Switch with errors.As / errors.Is (they also see through
// errors wrapped with %w):
func statusFor(err error) int {
	var ve *chat.ValidationError
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		return http.StatusRequestEntityTooLarge
	case errors.As(err, &ve):
		return http.StatusBadRequest
	case errors.Is(err, chat.ErrUserNotFound), errors.Is(err, chat.ErrRoomNotFound):
		return http.StatusNotFound
	case errors.Is(err, chat.ErrAlreadyExists):
		return http.StatusConflict
	case errors.Is(err, chat.ErrNotMember):
		return http.StatusForbidden
	case errors.Is(err, service.ErrUnauthenticated), errors.Is(err, service.ErrBadCredentials):
		return http.StatusUnauthorized
	case errors.Is(err, service.ErrLiveOff):
		return http.StatusServiceUnavailable
	case errors.Is(err, ratelimit.ErrLimited):
		return http.StatusTooManyRequests
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	case errors.Is(err, context.Canceled):
		return statusClientClosedRequest
	default:
		return http.StatusInternalServerError
	}
}

// invalid builds a validation error for request problems the domain doesn't
// know about, like a bad ?limit= value.
func invalid(field, reason string) error {
	return &chat.ValidationError{Field: field, Reason: reason}
}

// bodyError turns a failure to read or decode the request body into an
// error for writeError: a body over the LimitBody cap stays as it is (so
// statusFor answers 413), anything else is a 400 for the "body" field.
func bodyError(err error) error {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return err
	}
	return invalid("body", err.Error())
}
