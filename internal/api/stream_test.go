package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"chatserver/internal/chat"
	"chatserver/internal/hub"
	"chatserver/internal/store"
)

// TODO 46: postMessage must hand every stored message to a.publish.
func TestPostPublishes(t *testing.T) {
	got := make(chan chat.Message, 1)
	a := New(store.NewMemoryStore(), 1000).WithHub(nil, func(m chat.Message) { got <- m })
	s := &testServer{t: t, router: a.Router()}

	u := s.mustUser("alice")
	r := s.mustRoom(u, "general")
	expectStatus(t, s.do("POST", "/rooms/"+r+"/members", nil, u), http.StatusNoContent, "join")
	expectStatus(t, s.do("POST", "/rooms/"+r+"/messages", map[string]string{"text": "hi"}, u), http.StatusCreated, "post")

	select {
	case m := <-got:
		if m.Text != "hi" || string(m.Room) != r {
			t.Errorf("published %+v", m)
		}
	case <-time.After(time.Second):
		t.Fatal("postMessage didn't call a.publish (TODO 46)")
	}

	// A failed post must NOT be published.
	expectStatus(t, s.do("POST", "/rooms/"+r+"/messages", map[string]string{"text": "   "}, u), http.StatusBadRequest, "blank")
	select {
	case m := <-got:
		t.Errorf("a rejected message was published: %+v", m)
	case <-time.After(50 * time.Millisecond):
	}
}

// The full live path: POST -> pool (filters banned words) -> hub -> SSE stream.
// Needs TODOs 39-47.
func TestLiveStream(t *testing.T) {
	st := store.NewMemoryStore()
	h := hub.NewHub(16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.Run(ctx)

	pool := hub.NewPool(func(s string) bool { return strings.Contains(s, "spam") }, h.Publish, 16)
	pool.Start(1)
	defer pool.Stop()

	a := New(st, 1000).WithHub(h, pool.Submit)
	srv := httptest.NewServer(a.Router()) // a real server on a random port
	defer srv.Close()
	s := &testServer{t: t, router: a.Router()}

	u := s.mustUser("alice")
	r := s.mustRoom(u, "general")
	expectStatus(t, s.do("POST", "/rooms/"+r+"/members", nil, u), http.StatusNoContent, "join")

	// Open the stream like a browser would.
	reqCtx, stopStream := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopStream()
	req, _ := http.NewRequestWithContext(reqCtx, "GET", srv.URL+"/rooms/"+r+"/stream", nil)
	req.Header.Set("X-User-ID", u)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("opening the stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status %d, want 200 (is the route registered? TODO 47)", resp.StatusCode)
	}
	lines := bufio.NewReader(resp.Body)
	if first, _ := lines.ReadString('\n'); !strings.HasPrefix(first, ": connected") {
		t.Fatalf("first line %q, want the ': connected' comment", first)
	}

	post := func(text string) {
		body, _ := json.Marshal(map[string]string{"text": text})
		req, _ := http.NewRequest("POST", srv.URL+"/rooms/"+r+"/messages", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-ID", u)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("post %q: status %d", text, res.StatusCode)
		}
	}

	// readEvent returns the next message's text from the stream.
	readEvent := func() string {
		t.Helper()
		for {
			line, err := lines.ReadString('\n')
			if err != nil {
				t.Fatalf("stream ended or timed out before a message arrived: %v", err)
			}
			if data, ok := strings.CutPrefix(line, "data: "); ok {
				var m messageDTO
				if err := json.Unmarshal([]byte(data), &m); err != nil {
					t.Fatalf("bad event data %q: %v", data, err)
				}
				return m.Text
			}
		}
	}

	post("hello live")
	if got := readEvent(); got != "hello live" {
		t.Errorf("first event = %q, want %q", got, "hello live")
	}
	post("buy spam now")
	if got := readEvent(); got != "[removed]" {
		t.Errorf("second event = %q, want [removed] (the pool filters banned words)", got)
	}

	// History keeps the original text: moderation only applies to the live feed.
	hist := decode[[]messageDTO](t, s.do("GET", "/rooms/"+r+"/messages", nil, u))
	if len(hist) != 2 || hist[1].Text != "buy spam now" {
		t.Errorf("history = %+v", hist)
	}
}
