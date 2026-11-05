package main

import (
	"context"
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
func watch(ctx context.Context, client pb.ChatServiceClient, user, room string, ready chan<- error, received *atomic.Int64) {
	// 1. Open the stream, then wait for the server's header.
	stream, err := client.Watch(as(ctx, user), &pb.WatchRequest{RoomId: room})
	if err == nil {
		var md metadata.MD
		md, err = stream.Header()
		if err == nil && md == nil {
			// The call ended before any header: Recv returns the reason.
			_, err = stream.Recv()
		}
	}

	// 2. Report: nil = watching, an error = why not. Always exactly once.
	ready <- err
	if err != nil {
		return
	}

	// 3. Count messages until the stream ends (ctx cancelled = test over).
	for {
		if _, err := stream.Recv(); err != nil {
			return
		}
		received.Add(1)
	}
}

// posterResult is one poster's measurements.
type posterResult struct {
	latencies []time.Duration
	failed    int64
}

// post is one poster: it posts rate messages a second until ctx ends, and
// times every call.
func post(ctx context.Context, client pb.ChatServiceClient, user, room string, rate float64) posterResult {
	var res posterResult // one poster's measurements
	tick := time.NewTicker(time.Duration(float64(time.Second) / rate))
	defer tick.Stop()
	ctx = as(ctx, user)
	for n := 1; ; n++ {
		select {
		case <-ctx.Done():
			return res
		case <-tick.C:
		}
		start := time.Now() // time the call, not the tick
		_, err := client.PostMessage(ctx, &pb.PostMessageRequest{
			RoomId: room,
			Text:   fmt.Sprintf("load %s #%d", user, n),
		})
		if err != nil {
			if ctx.Err() != nil {
				return res
			}
			res.failed++
			continue
		}
		res.latencies = append(res.latencies, time.Since(start))
	}
}
