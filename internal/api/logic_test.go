package api

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"chatserver/internal/chat"
)

// These tests don't start a server: they test the plain-Go parts.
// Run just them with:  go test ./internal/api -run 'StatusFor|MessageDTO|Limiter' -v

func TestStatusFor(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"validation", &chat.ValidationError{Field: "name", Reason: "too short"}, http.StatusBadRequest},
		{"wrapped validation", fmt.Errorf("create: %w", &chat.ValidationError{Field: "name"}), http.StatusBadRequest},
		{"user not found", fmt.Errorf("user %q: %w", "u-9", chat.ErrUserNotFound), http.StatusNotFound},
		{"room not found", fmt.Errorf("room %q: %w", "r-9", chat.ErrRoomNotFound), http.StatusNotFound},
		{"already exists", fmt.Errorf("room %q: %w", "dev", chat.ErrAlreadyExists), http.StatusConflict},
		{"not member", chat.ErrNotMember, http.StatusForbidden},
		{"unknown", errors.New("disk on fire"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusFor(tt.err); got != tt.want {
				t.Errorf("statusFor(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

func TestToMessageDTO(t *testing.T) {
	at := time.Date(2026, 10, 28, 15, 0, 0, 0, time.UTC)
	m := chat.Message{
		Meta:   chat.Meta{ID: "m-7", CreatedAt: at},
		Room:   "r-1",
		Author: "u-2",
		Kind:   chat.KindText,
		Text:   "hello",
	}
	got := toMessageDTO(m)
	want := messageDTO{ID: "m-7", Room: "r-1", Author: "u-2", Kind: "text", Text: "hello", CreatedAt: at}
	if got != want {
		t.Errorf("toMessageDTO =\n %+v\nwant\n %+v", got, want)
	}
}

func TestLimiter(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	l := NewLimiter(2, clock)

	if !l.Allow("u-1") || !l.Allow("u-1") {
		t.Fatal("first 2 requests in a second should be allowed")
	}
	if l.Allow("u-1") {
		t.Error("3rd request in the same second should be refused")
	}
	if !l.Allow("u-2") {
		t.Error("each user has their own limit: u-2 should be allowed")
	}

	now = now.Add(999 * time.Millisecond)
	if l.Allow("u-1") {
		t.Error("still inside the same 1-second window: should be refused")
	}

	now = now.Add(1 * time.Millisecond) // exactly 1s after the window started
	if !l.Allow("u-1") {
		t.Error("a new window should start after one second")
	}
}
