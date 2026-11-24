package main

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"chatserver/internal/grpcapi"
	"chatserver/internal/hub"
	"chatserver/internal/pb"
	"chatserver/internal/ratelimit"
	"chatserver/internal/service"
	"chatserver/internal/store"
)

// startServer runs the real gRPC server in memory (bufconn, as in grpcapi's
// tests) and returns a client for it, plus the service for setting up.
func startServer(t *testing.T) (pb.ChatServiceClient, *service.Service) {
	t.Helper()
	s := store.NewMemoryStore()
	h := hub.NewHub(64)
	hubCtx, stopHub := context.WithCancel(context.Background())
	go h.Run(hubCtx)

	svc := service.New(s).WithHub(h, h.Publish).WithPasswordCost(bcrypt.MinCost)
	gs := grpc.NewServer(grpc.UnaryInterceptor(grpcapi.UnaryAuth(svc, ratelimit.Limits{})), grpc.StreamInterceptor(grpcapi.StreamAuth(svc, ratelimit.Limits{})))
	pb.RegisterChatServiceServer(gs, grpcapi.New(svc))
	lis := bufconn.Listen(1 << 20)
	go gs.Serve(lis)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		conn.Close()
		gs.Stop()
		stopHub()
	})
	return pb.NewChatServiceClient(conn), svc
}

// users signs up n users and returns their session tokens.
func users(t *testing.T, svc *service.Service, prefix string, n int) []string {
	t.Helper()
	var tokens []string
	for i := range n {
		_, token, err := svc.Register(t.Context(), prefix+string(rune('a'+i)), "load-test-password")
		if err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, token)
	}
	return tokens
}

// A short, gentle load where nothing should be lost.
func TestRun(t *testing.T) {
	client, svc := startServer(t)
	posters := users(t, svc, "poster", 2)
	watchers := users(t, svc, "watcher", 5)
	room, err := svc.CreateRoom(t.Context(), "loadroom", 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range append(posters, watchers...) { // watching needs membership too
		u, err := svc.Authenticate(t.Context(), token)
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.JoinRoom(t.Context(), u.ID, room.ID); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg := Config{Room: string(room.ID), Posters: posters, Watchers: watchers, Rate: 40, Duration: 500 * time.Millisecond}

	done := make(chan struct{})
	var res Result
	go func() {
		defer close(done)
		res, err = run(ctx, client, cfg)
	}()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("run didn't finish: is a watcher or poster ignoring ctx?")
	}
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// 2 posters x 40/s x 0.5s = about 40 posts.
	if res.Posted < 20 || res.Posted > 50 {
		t.Errorf("posted %d, want about 40 (2 posters at 40/s for 0.5s)", res.Posted)
	}
	if res.Failed != 0 {
		t.Errorf("%d posts failed", res.Failed)
	}
	if int64(len(res.Latencies)) != res.Posted {
		t.Errorf("%d latencies for %d posts: record one per successful post", len(res.Latencies), res.Posted)
	}
	for _, l := range res.Latencies {
		if l < 0 {
			t.Fatalf("a negative latency of %v: measure with time.Now() before and time.Since(start) after", l)
		}
	}
	if want := res.Posted * int64(len(watchers)); res.Received != want {
		t.Errorf("received %d, want %d (every watcher gets every post)", res.Received, want)
	}
}

func TestRunReportsAWatcherThatCantStart(t *testing.T) {
	client, svc := startServer(t)
	cfg := Config{Room: "r-404", Posters: users(t, svc, "poster", 1), Watchers: users(t, svc, "watcher", 2), Rate: 10, Duration: 100 * time.Millisecond}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := run(ctx, client, cfg)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("run with a missing room = %v, want an error saying the room wasn't found (watch must send the error on ready)", err)
	}
}
