package grpcapi

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"chatserver/internal/chat"
)

func TestToStatus(t *testing.T) {
	tests := []struct {
		err  error
		want codes.Code
	}{
		{&chat.ValidationError{Field: "name", Reason: "too short"}, codes.InvalidArgument},
		{fmt.Errorf("user %q: %w", "u-9", chat.ErrUserNotFound), codes.NotFound},
		{fmt.Errorf("room %q: %w", "r-9", chat.ErrRoomNotFound), codes.NotFound},
		{chat.ErrAlreadyExists, codes.AlreadyExists},
		{chat.ErrNotMember, codes.PermissionDenied},
		{errors.New("disk on fire"), codes.Internal},
	}
	for _, tt := range tests {
		got := toStatus(tt.err)
		if code := status.Code(got); code != tt.want {
			t.Errorf("toStatus(%v) has code %v, want %v", tt.err, code, tt.want)
		}
		if got != nil && status.Convert(got).Message() != tt.err.Error() {
			t.Errorf("toStatus(%v) message = %q, want the original error text", tt.err, status.Convert(got).Message())
		}
	}
	if toStatus(nil) != nil {
		t.Error("toStatus(nil) must be nil")
	}
}
