package pbconv

import (
	"testing"
	"time"

	"chatserver/internal/chat"
	"chatserver/internal/pb"
)

// These were grpcapi's TestConvert; they moved here with the code.

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
		if got := User(&chat.User{ID: "u-1", Name: "alice", Role: tt.in}).GetRole(); got != tt.want {
			t.Errorf("role %v -> %v, want %v", tt.in, got, tt.want)
		}
	}

	u := User(&chat.User{ID: "u-7", Name: "alice", Role: chat.RoleMember})
	if u.GetId() != "u-7" || u.GetName() != "alice" {
		t.Errorf("User = %v", u)
	}

	r := chat.NewRoom("r-3", "general", 10)
	if err := r.Join("u-1"); err != nil {
		t.Fatal(err)
	}
	if pr := Room(r); pr.GetId() != "r-3" || pr.GetName() != "general" || pr.GetMembers() != 1 {
		t.Errorf("Room = %v, want id r-3, name general, 1 member", pr)
	}

	at := time.Date(2026, 11, 4, 9, 30, 0, 0, time.UTC)
	m := chat.Message{Meta: chat.Meta{ID: "m-5", CreatedAt: at}, Room: "r-3", Author: "u-7", Kind: chat.KindText, Text: "hi"}
	pm := Message(m)
	if pm.GetId() != "m-5" || pm.GetRoomId() != "r-3" || pm.GetAuthorId() != "u-7" || pm.GetText() != "hi" {
		t.Errorf("Message = %v", pm)
	}
	if pm.GetKind() != pb.MessageKind_MESSAGE_KIND_TEXT {
		t.Errorf("kind = %v, want MESSAGE_KIND_TEXT", pm.GetKind())
	}
	if pm.GetCreatedAtUnixMilli() != at.UnixMilli() {
		t.Errorf("created_at = %d, want %d (use UnixMilli)", pm.GetCreatedAtUnixMilli(), at.UnixMilli())
	}

	if got := Messages([]chat.Message{m, m}); len(got) != 2 {
		t.Errorf("Messages returned %d messages, want 2", len(got))
	}
	if got := Messages(nil); got == nil || len(got) != 0 {
		t.Errorf("Messages(nil) = %v, want an empty (non-nil) slice", got)
	}
}

