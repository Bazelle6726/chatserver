package grpcapi

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"chatserver/internal/chat"
	"chatserver/internal/pb"
)

// These two tests call the functions directly: no network, no server.

func TestConvert(t *testing.T) {
	roles := []struct {
		in   chat.Role
		want pb.Role
	}{
		{chat.RoleGuest, pb.Role_ROLE_GUEST},
		{chat.RoleMember, pb.Role_ROLE_MEMBER},
		{chat.RoleAdmin, pb.Role_ROLE_ADMIN},
		{chat.Role(99), pb.Role_ROLE_UNSPECIFIED},
	}
	for _, tt := range roles {
		if got := toPBUser(&chat.User{ID: "u-1", Name: "alice", Role: tt.in}).GetRole(); got != tt.want {
			t.Errorf("role %v -> %v, want %v", tt.in, got, tt.want)
		}
	}

	u := toPBUser(&chat.User{ID: "u-7", Name: "alice", Role: chat.RoleMember})
	if u.GetId() != "u-7" || u.GetName() != "alice" {
		t.Errorf("toPBUser = %v", u)
	}

	r := chat.NewRoom("r-3", "general", 10)
	if err := r.Join("u-1"); err != nil {
		t.Fatal(err)
	}
	if pr := toPBRoom(r); pr.GetId() != "r-3" || pr.GetName() != "general" || pr.GetMembers() != 1 {
		t.Errorf("toPBRoom = %v, want id r-3, name general, 1 member", pr)
	}

	at := time.Date(2026, 11, 4, 9, 30, 0, 0, time.UTC)
	m := chat.Message{Meta: chat.Meta{ID: "m-5", CreatedAt: at}, Room: "r-3", Author: "u-7", Kind: chat.KindText, Text: "hi"}
	pm := toPBMessage(m)
	if pm.GetId() != "m-5" || pm.GetRoomId() != "r-3" || pm.GetAuthorId() != "u-7" || pm.GetText() != "hi" {
		t.Errorf("toPBMessage = %v", pm)
	}
	if pm.GetKind() != pb.MessageKind_MESSAGE_KIND_TEXT {
		t.Errorf("kind = %v, want MESSAGE_KIND_TEXT", pm.GetKind())
	}
	if pm.GetCreatedAtUnixMilli() != at.UnixMilli() {
		t.Errorf("created_at = %d, want %d (use UnixMilli)", pm.GetCreatedAtUnixMilli(), at.UnixMilli())
	}

	if got := toPBMessages([]chat.Message{m, m}); len(got) != 2 {
		t.Errorf("toPBMessages returned %d messages, want 2", len(got))
	}
	if got := toPBMessages(nil); got == nil || len(got) != 0 {
		t.Errorf("toPBMessages(nil) = %v, want an empty (non-nil) slice", got)
	}
}

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
