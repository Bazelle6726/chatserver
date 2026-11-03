package hub

import (
	"context"
	"fmt"
	"testing"
	"time"

	"chatserver/internal/chat"
)

// Concurrency bugs often show up as a test that hangs forever. Every wait in
// these tests has a timeout, so a mistake fails with a message instead.
const wait = time.Second

func startHub(t *testing.T, buffer int) (*Hub, context.CancelFunc) {
	t.Helper()
	h := NewHub(buffer)
	ctx, cancel := context.WithCancel(context.Background())
	go h.Run(ctx)
	t.Cleanup(cancel)
	return h, cancel
}

func msg(room chat.RoomID, text string) chat.Message {
	return chat.Message{Room: room, Kind: chat.KindText, Text: text}
}

// receive waits for one message, or fails.
func receive(t *testing.T, c *Client, who string) chat.Message {
	t.Helper()
	select {
	case m, ok := <-c.Out():
		if !ok {
			t.Fatalf("%s: channel closed, expected a message", who)
		}
		return m
	case <-time.After(wait):
		t.Fatalf("%s: no message within %v (is Run's select loop working?)", who, wait)
	}
	return chat.Message{}
}

// expectNothing checks that no message arrives soon.
func expectNothing(t *testing.T, c *Client, who string) {
	t.Helper()
	select {
	case m, ok := <-c.Out():
		if ok {
			t.Fatalf("%s: got %q, but it isn't in that room", who, m.Text)
		}
		t.Fatalf("%s: channel was closed unexpectedly", who)
	case <-time.After(50 * time.Millisecond):
	}
}

// expectClosed waits for the channel to be closed (draining anything buffered).
func expectClosed(t *testing.T, c *Client, who string) {
	t.Helper()
	deadline := time.After(wait)
	for {
		select {
		case _, ok := <-c.Out():
			if !ok {
				return
			}
		case <-deadline:
			t.Fatalf("%s: channel not closed within %v", who, wait)
		}
	}
}

func TestBroadcastReachesOnlyThatRoom(t *testing.T) {
	h, _ := startHub(t, 8)
	alice := h.Join("u-1", "r-1")
	bob := h.Join("u-2", "r-1")
	carol := h.Join("u-3", "r-2")

	h.Publish(msg("r-1", "hello r-1"))

	if m := receive(t, alice, "alice"); m.Text != "hello r-1" {
		t.Errorf("alice got %q", m.Text)
	}
	if m := receive(t, bob, "bob"); m.Text != "hello r-1" {
		t.Errorf("bob got %q", m.Text)
	}
	expectNothing(t, carol, "carol (room r-2)")
}

func TestMessagesArriveInOrder(t *testing.T) {
	h, _ := startHub(t, 16)
	c := h.Join("u-1", "r-1")
	for i := 1; i <= 10; i++ {
		h.Publish(msg("r-1", fmt.Sprint(i)))
	}
	for i := 1; i <= 10; i++ {
		if m := receive(t, c, "client"); m.Text != fmt.Sprint(i) {
			t.Fatalf("message %d: got %q (hub must deliver in the order published)", i, m.Text)
		}
	}
}

func TestLeaveClosesOutbox(t *testing.T) {
	h, _ := startHub(t, 8)
	a := h.Join("u-1", "r-1")
	b := h.Join("u-2", "r-1")

	h.Leave(a)
	expectClosed(t, a, "a after Leave")

	h.Leave(a) // twice must not panic (closing a closed channel would)

	h.Publish(msg("r-1", "still here"))
	if m := receive(t, b, "b"); m.Text != "still here" {
		t.Errorf("b got %q", m.Text)
	}
}

func TestSlowClientIsDropped(t *testing.T) {
	h, _ := startHub(t, 2)       // each outbox holds 2 messages
	slow := h.Join("u-1", "r-1") // never reads
	fast := h.Join("u-2", "r-1") // reads every message

	for i := 1; i <= 3; i++ {
		h.Publish(msg("r-1", fmt.Sprint(i)))
		if m := receive(t, fast, "fast"); m.Text != fmt.Sprint(i) {
			t.Fatalf("fast got %q, want %d", m.Text, i)
		}
	}

	// slow's buffer filled with 1 and 2; message 3 found it full -> dropped.
	deadline := time.Now().Add(wait)
	for h.Dropped() != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("Dropped() = %d, want 1 (fanOut must not block on a full outbox)", h.Dropped())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if m := receive(t, slow, "slow"); m.Text != "1" {
		t.Errorf("slow's first buffered message = %q, want 1", m.Text)
	}
	if m := receive(t, slow, "slow"); m.Text != "2" {
		t.Errorf("slow's second buffered message = %q, want 2", m.Text)
	}
	expectClosed(t, slow, "slow after being dropped")
}

func TestShutdownClosesEveryone(t *testing.T) {
	h, cancel := startHub(t, 8)
	a := h.Join("u-1", "r-1")
	b := h.Join("u-2", "r-2")

	cancel()
	expectClosed(t, a, "a after shutdown")
	expectClosed(t, b, "b after shutdown")

	// After shutdown nothing may block.
	done := make(chan struct{})
	go func() {
		h.Publish(msg("r-1", "late"))
		late := h.Join("u-3", "r-1")
		h.Leave(late)
		<-late.Out() // closed straight away
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(wait):
		t.Fatal("Publish/Join/Leave blocked after the hub stopped")
	}
}
