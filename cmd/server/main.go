// Command server runs the chat server: the HTTP API on http://localhost:8080
// and the gRPC API on localhost:9090. Both share one store, hub and pool.
//
// Press Ctrl+C to stop it. It shuts down gracefully: in-flight requests
// finish, live streams close, and every queued message is delivered first.
package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"chatserver/internal/api"
	"chatserver/internal/chat"
	"chatserver/internal/grpcapi"
	"chatserver/internal/hub"
	"chatserver/internal/pb"
	"chatserver/internal/store"
)

func main() {
	// ctx is cancelled when you press Ctrl+C. Everything long-running watches it.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// pprof: profiling endpoints, on their own port and only on localhost.
	go func() {
		log.Println("pprof on http://localhost:6060/debug/pprof/")
		if err := http.ListenAndServe("localhost:6060", nil); err != nil {
			log.Println("pprof:", err)
		}
	}()

	s := store.NewMemoryStore()

	// The hub: one goroutine that knows who's watching which room.
	h := hub.NewHub(64)
	hubDone := make(chan struct{})
	go func() {
		h.Run(ctx)
		close(hubDone)
	}()

	// The moderation pool: workers filter banned words, then publish to the hub.
	// mod.IsBanned is a "method value": a function bound to mod.
	mod := chat.Admin{User: &chat.User{Name: "moderator", Role: chat.RoleAdmin}, BannedWords: []string{"spam", "scam"}}
	pool := hub.NewPool(mod.IsBanned, h.Publish, 256)
	pool.Start(runtime.NumCPU())

	// HTTP (stages 2-3)
	a := api.New(s, 20).WithHub(h, pool.Submit)
	srv := &http.Server{Addr: ":8080", Handler: a.Router()}
	go func() {
		log.Println("HTTP listening on http://localhost:8080")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	// gRPC (stage 4): the same store, hub and pool behind a second front door.
	lis, err := net.Listen("tcp", ":9090")
	if err != nil {
		log.Fatal(err)
	}
	gs := grpc.NewServer(
		grpc.UnaryInterceptor(grpcapi.UnaryAuth(s)),
		grpc.StreamInterceptor(grpcapi.StreamAuth(s)),
	)
	pb.RegisterChatServiceServer(gs, grpcapi.New(s, h, pool.Submit))
	reflection.Register(gs) // lets tools like grpcurl list the service without the .proto
	go func() {
		log.Println("gRPC listening on localhost:9090")
		if err := gs.Serve(lis); err != nil {
			log.Fatal(err)
		}
	}()

	<-ctx.Done() // wait for Ctrl+C
	log.Println("shutting down...")

	// The ORDER matters:
	// 1. The hub stops (it watches ctx) and closes every client's outbox,
	//    so every /stream handler, Watch and Converse returns.
	<-hubDone

	// 2. Stop both front doors and wait for calls already running. After this
	//    nothing can call pool.Submit any more.
	//    GracefulStop waits for every running RPC with no time limit, and an
	//    Import whose client never finishes would wait forever. So give it 5s,
	//    then Stop, which cuts the remaining calls off.
	grpcStopped := make(chan struct{})
	go func() {
		gs.GracefulStop()
		close(grpcStopped)
	}()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Println("HTTP shutdown:", err)
	}
	select {
	case <-grpcStopped:
	case <-shutdownCtx.Done():
		gs.Stop()
		<-grpcStopped
	}

	// 3. Only now stop the pool: closing its jobs channel earlier could make a
	//    late request panic with "send on closed channel".
	pool.Stop()

	log.Printf("bye (dropped %d slow clients)", h.Dropped())
}
