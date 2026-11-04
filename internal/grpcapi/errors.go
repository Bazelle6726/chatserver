package grpcapi

import (
	"chatserver/internal/chat"
)

// toStatus turns a domain error into a gRPC status error.
//
// TODO(50): this is statusFor from api/errors.go again, with gRPC codes
// (package google.golang.org/grpc/codes) instead of HTTP status codes:
//
//	nil                    -> nil (check this first)
//	*chat.ValidationError  -> codes.InvalidArgument   (errors.As)
//	ErrUserNotFound,
//	ErrRoomNotFound        -> codes.NotFound          (errors.Is)
//	ErrAlreadyExists       -> codes.AlreadyExists
//	ErrNotMember           -> codes.PermissionDenied
//	anything else          -> codes.Internal
//
// Build each one with status.Error(code, err.Error()) from package
// google.golang.org/grpc/status, so the client sees the original message.
// Add the imports you need (errors, codes, status).
func toStatus(err error) error {
	return err
}

// invalid builds a validation error for a request field.
func invalid(field, reason string) error {
	return &chat.ValidationError{Field: field, Reason: reason}
}
