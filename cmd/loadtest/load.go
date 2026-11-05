package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/metadata"

	"chatserver/internal/pb"
)

// Config describes one load test.
type Config struct {
	Room     string
	Posters  []string      // user IDs that post (members of Room)
	Watchers []string      // user IDs that watch Room
	Rate     float64       // posts per second, per poster
	Duration time.Duration // how long the posters post for
}

// Result is what a load test measured.
type Result struct {
	Posted    int64           // successful posts
	Failed    int64           // posts that returned an error
	Received  int64           // messages received, summed over every watcher
	Latencies []time.Duration // one per successful post
	Elapsed   time.Duration   // how long the posting phase took
}

// as returns a context that makes calls as user.
func as(ctx context.Context, user string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "x-user-id", user)
}

// run carries out the test in three phases:
//
//  1. start every watcher, and wait until each one is really watching
//  2. start every poster; wait for them all to finish (cfg.Duration)
//  3. give the last messages a moment to arrive, then stop the watchers
func run(ctx context.Context, client pb.ChatServiceClient, cfg Config) (Result, error) {
	var res Result
	var received atomic.Int64 // every watcher adds to it, so it must be atomic

	// 1. Watchers.
	watchCtx, stopWatching := context.WithCancel(ctx)
	defer stopWatching()
	var watchers sync.WaitGroup
	ready := make(chan error, len(cfg.Watchers))
	for _, user := range cfg.Watchers {
		watchers.Add(1)
		go func() {
			defer watchers.Done()
			watch(watchCtx, client, user, cfg.Room, ready, &received)
		}()
	}
	for range cfg.Watchers {
		if err := <-ready; err != nil {
			return res, fmt.Errorf("starting a watcher: %w", err)
		}
	}

	// 2. Posters. Each one fills in its own slot of results, so they share
	// nothing while running and need no lock.
	postCtx, stopPosting := context.WithTimeout(ctx, cfg.Duration)
	defer stopPosting()
	results := make([]posterResult, len(cfg.Posters))
	var posters sync.WaitGroup
	start := time.Now()
	for i, user := range cfg.Posters {
		posters.Add(1)
		go func() {
			defer posters.Done()
			results[i] = post(postCtx, client, user, cfg.Room, cfg.Rate)
		}()
	}
	posters.Wait()
	res.Elapsed = time.Since(start)
	for _, r := range results {
		res.Posted += int64(len(r.latencies))
		res.Failed += r.failed
		res.Latencies = append(res.Latencies, r.latencies...)
	}

	// 3. Wait (up to 2s) for every watcher to receive every message.
	want := res.Posted * int64(len(cfg.Watchers))
	deadline := time.Now().Add(2 * time.Second)
	for received.Load() < want && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	stopWatching()
	watchers.Wait()
	res.Received = received.Load()
	return res, nil
}

// watch is one watcher: it opens a Watch stream, reports on ready once it's
// really watching (or why it couldn't), then counts messages until ctx ends.
//
// TODO(71):
//  1. stream, err := client.Watch(as(ctx, user), &pb.WatchRequest{RoomId: room})
//  2. if that worked, wait for the server's header: md, err := stream.Header().
//     Watch sends it once it has joined the hub, so after this nothing posted
//     can be missed. (Watch that variable shadowing: use = for err here.)
//  3. gRPC quirk: if the call ended before any header (say the room doesn't
//     exist), Header returns nil, nil. Then call stream.Recv(), whose error
//     is the real reason.
//  4. send the result on ready, error or nil: ready <- err. run waits for
//     one value from every watcher before posting starts. Return if it failed.
//  5. loop: stream.Recv(); on error return (ctx was cancelled: the test is
//     over), else received.Add(1)
//
// Check (with TODO 72 too): go test ./cmd/loadtest -v
func watch(ctx context.Context, client pb.ChatServiceClient, user, room string, ready chan<- error, received *atomic.Int64) {
	ready <- errors.New("TODO(71): watch")
}

// posterResult is one poster's measurements.
type posterResult struct {
	latencies []time.Duration
	failed    int64
}

// post is one poster: it posts rate messages a second until ctx ends, and
// times every call.
//
// TODO(72):
//   - var res posterResult
//   - a ticker: time.NewTicker(time.Duration(float64(time.Second) / rate)),
//     and defer tick.Stop() (a ticker that's never stopped keeps running)
//   - ctx = as(ctx, user), so every post is made as this user
//   - loop, counting n from 1:
//   - select: <-ctx.Done() -> return res; <-tick.C -> carry on
//   - start := time.Now(), then client.PostMessage(ctx, ...) with RoomId room
//     and any text, e.g. fmt.Sprintf("load %s #%d", user, n)
//   - on error: if ctx.Err() != nil, return res (the time ran out during the
//     call, which isn't a failure); otherwise res.failed++ and continue
//   - on success: append time.Since(start) to res.latencies
func post(ctx context.Context, client pb.ChatServiceClient, user, room string, rate float64) posterResult {
	<-ctx.Done()
	return posterResult{}
}
