package grpcapi

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"

	"chatserver/internal/moderation"
	"chatserver/internal/pb"
	"chatserver/internal/store"
)

// History must hide banned words the same way the HTTP API and the live feed
// do, or gRPC clients could read what everyone else sees as "[removed]".
func TestHistoryHidesBannedWords(t *testing.T) {
	s := store.NewMemoryStore()
	u, _ := s.CreateUser(t.Context(), "alice")
	r, _ := s.CreateRoom(t.Context(), "general", 10)
	if err := r.Join(u.ID); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"hello", "cheap scam here", "scampi"} {
		if _, err := s.PostMessage(t.Context(), r.ID, u.ID, text); err != nil {
			t.Fatal(err)
		}
	}

	srv := New(s, nil, nil).WithModeration(moderation.New("scam").IsBanned)
	ctx := withUser(context.Background(), u) // what the auth interceptor does
	resp, err := srv.History(ctx, &pb.HistoryRequest{RoomId: string(r.ID)})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"hello", "[removed]", "scampi"}
	if len(resp.Messages) != len(want) {
		t.Fatalf("got %d messages, want %d", len(resp.Messages), len(want))
	}
	for i, m := range resp.Messages {
		if m.Text != want[i] {
			t.Errorf("message %d = %q, want %q", i, m.Text, want[i])
		}
	}

	// The store must still hold the original.
	if got := r.LastN(3)[1].Text; got != "cheap scam here" {
		t.Errorf("stored text = %q, want the original", got)
	}
}

// History and Watch need membership, the same as posting.
func TestReadingNeedsMembership(t *testing.T) {
	e := newEnv(t)
	alice := e.user("alice")
	eve := e.user("eve")
	room := e.room("general", alice)

	_, err := e.client.History(e.as(eve), &pb.HistoryRequest{RoomId: room})
	wantCode(t, err, codes.PermissionDenied, "History by a non-member")

	stream, err := e.client.Watch(e.as(eve), &pb.WatchRequest{RoomId: room})
	if err == nil {
		_, err = stream.Recv() // a stream's error arrives with its first Recv
	}
	wantCode(t, err, codes.PermissionDenied, "Watch by a non-member")

	if _, err := e.client.History(e.as(alice), &pb.HistoryRequest{RoomId: room}); err != nil {
		t.Errorf("History by a member: %v", err)
	}
}
