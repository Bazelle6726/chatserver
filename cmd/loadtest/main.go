// Command loadtest puts the chat server under load over gRPC: many users
// watching one room while others post to it. It reports how many messages
// went through, how long each post took, and whether every watcher got every
// message.
//
// Start the server first (go run ./cmd/server), then:
//
//	go run ./cmd/loadtest
//	go run ./cmd/loadtest -watchers 500 -posters 20 -rate 50 -duration 30s
//
// gRPC has no rate limiter (that's an HTTP middleware), so the load is only
// limited by -rate.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"slices"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"chatserver/internal/pb"
)

func main() {
	addr := flag.String("addr", "localhost:9090", "the gRPC server")
	watchers := flag.Int("watchers", 100, "users watching the room")
	posters := flag.Int("posters", 10, "users posting to the room")
	rate := flag.Float64("rate", 20, "posts per second, per poster")
	duration := flag.Duration("duration", 10*time.Second, "how long to post for")
	history := flag.Int("history", 1000, "the room's max history (1-1000)")
	flag.Parse()

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	client := pb.NewChatServiceClient(conn)
	ctx := context.Background()

	cfg, err := setup(ctx, client, *posters, *watchers, *history)
	if err != nil {
		log.Fatal("setup: ", err)
	}
	cfg.Rate = *rate
	cfg.Duration = *duration

	fmt.Printf("room %s: %d watchers, %d posters at %.0f/s each, for %v\n",
		cfg.Room, len(cfg.Watchers), len(cfg.Posters), cfg.Rate, cfg.Duration)
	res, err := run(ctx, client, cfg)
	if err != nil {
		log.Fatal(err)
	}
	report(res, len(cfg.Watchers))
}

// setup creates the users and the room, and makes every poster a member.
// Names get a fresh tag each run, so you can run it again without clashes.
func setup(ctx context.Context, client pb.ChatServiceClient, posters, watchers, history int) (Config, error) {
	tag := fmt.Sprintf("lt%d", time.Now().Unix()%1000000)
	newUser := func(name string) (string, error) {
		u, err := client.CreateUser(ctx, &pb.CreateUserRequest{Name: name})
		return u.GetId(), err
	}

	var cfg Config
	for i := range posters {
		id, err := newUser(fmt.Sprintf("%sp%d", tag, i))
		if err != nil {
			return cfg, err
		}
		cfg.Posters = append(cfg.Posters, id)
	}
	for i := range watchers {
		id, err := newUser(fmt.Sprintf("%sw%d", tag, i))
		if err != nil {
			return cfg, err
		}
		cfg.Watchers = append(cfg.Watchers, id)
	}
	if len(cfg.Posters) == 0 {
		return cfg, fmt.Errorf("need at least one poster")
	}

	room, err := client.CreateRoom(as(ctx, cfg.Posters[0]), &pb.CreateRoomRequest{Name: tag, MaxHistory: int32(history)})
	if err != nil {
		return cfg, err
	}
	cfg.Room = room.GetId()
	for _, p := range cfg.Posters {
		if _, err := client.JoinRoom(as(ctx, p), &pb.JoinRoomRequest{RoomId: cfg.Room}); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

func report(res Result, watchers int) {
	slices.Sort(res.Latencies)
	secs := res.Elapsed.Seconds()
	fmt.Printf("\nposted     %d messages in %.1fs (%.0f/s), %d failed\n",
		res.Posted, secs, float64(res.Posted)/secs, res.Failed)
	fmt.Printf("latency    p50 %v   p95 %v   p99 %v   max %v\n",
		round(percentile(res.Latencies, 50)), round(percentile(res.Latencies, 95)),
		round(percentile(res.Latencies, 99)), round(percentile(res.Latencies, 100)))
	want := res.Posted * int64(watchers)
	pct := 100.0
	if want > 0 {
		pct = 100 * float64(res.Received) / float64(want)
	}
	fmt.Printf("delivered  %d of %d (%.1f%%)\n", res.Received, want, pct)
}

func round(d time.Duration) time.Duration { return d.Round(10 * time.Microsecond) }
