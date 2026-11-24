package grpcapi

import (
	"chatserver/internal/chat"
	"errors"
	"log"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// toStatus turns a domain error into a gRPC status error.
func toStatus(err error) error {
	if err == nil {
		return nil
	}
	var validationErr *chat.ValidationError
	if errors.As(err, &validationErr) {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	switch {
	case errors.Is(err, chat.ErrUserNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, chat.ErrRoomNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, chat.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, chat.ErrNotMember):
		return status.Error(codes.PermissionDenied, err.Error())
	default:
		// The server's own fault: log the details, don't send them.
		log.Printf("internal error: %v", err)
		return status.Error(codes.Internal, "internal error")
	}
}

// invalid builds a validation error for a request field.
func invalid(field, reason string) error {
	return &chat.ValidationError{Field: field, Reason: reason}
}
