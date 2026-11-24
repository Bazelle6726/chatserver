// Command server runs the chat server: the web app and HTTP API (by default
// on http://localhost:8080) and the gRPC API (localhost:9090). Both share one
// service, store, hub and pool.
//
//	go run ./cmd/server                       # the defaults
//	go run ./cmd/server -h                    # every setting
//	go run ./cmd/server -http-addr :8080      # also accept other machines
//
// Settings come from flags, or CHAT_* environment variables (see package
// config). Logs are structured (log/slog): text by default, or JSON with
// -log-format json for a log collector.
//
// Press Ctrl+C to stop it. It shuts down gracefully: live streams close,
// in-flight requests finish, and the worker pool drains its queue. Live
// streams close first, so messages still queued at that point are saved in
// the room history but have no watchers left to be pushed to.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"chatserver/internal/api"
	"chatserver/internal/config"
	"chatserver/internal/grpcapi"
	"chatserver/internal/hub"
	"chatserver/internal/moderation"
	"chatserver/internal/pb"
	"chatserver/internal/ratelimit"
	"chatserver/internal/service"
	"chatserver/internal/store"
	"chatserver/web"
)

func main() {
	cfg, warnings, err := config.Load(os.Args[1:], os.Getenv, os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return // -h: the flags have been printed
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(2)
	}

	logger := newLogger(cfg)
	// Make it the default too, so slog.Info and the old log package anywhere
	// in the program write through it.
	slog.SetDefault(logger)
	for _, w := range warnings {
		logger.Warn(w)
	}

	if err := run(cfg, logger); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

// newLogger builds the logger cfg asks for: human-readable text, or one JSON
// object per line for log collectors.
func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.LogFormat == "json" {
		return slog.New(slog.NewJSONHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stderr, opts))
}

// run starts everything, waits for Ctrl+C, and shuts down. It returns an
// error if something couldn't start.
func run(cfg config.Config, logger *slog.Logger) error {
	// ctx is cancelled when you press Ctrl+C. Everything long-running watches it.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// pprof: profiling endpoints, on their own address (localhost by default).
	if cfg.PprofAddr != "" {
		go func() {
			logger.Info("pprof listening", "url", "http://"+cfg.PprofAddr+"/debug/pprof/")
			if err := http.ListenAndServe(cfg.PprofAddr, nil); err != nil {
				logger.Error("pprof stopped", "err", err)
			}
		}()
	}

	s := store.NewMemoryStore()

	// The hub: one goroutine that knows who's watching which room.
	h := hub.NewHub(64)
	hubDone := make(chan struct{})
	go func() {
		h.Run(ctx)
		close(hubDone)
	}()

	// The moderation pool: workers filter banned words, then publish to the hub.
	// words.IsBanned is a "method value": a function bound to words.
	words, err := moderation.Load(cfg.BannedWordsPath)
	if err != nil {
		return err
	}
	pool := hub.NewPool(words.IsBanned, h.Publish, 256)
	pool.Start(runtime.NumCPU())

	// Rate limits, shared by both front doors so neither is a way round them.
	// Token buckets: N a second on average, in bursts of up to M.
	limits := ratelimit.Limits{
		PerUser: ratelimit.New(cfg.RequestsPerSecond, int(2*cfg.RequestsPerSecond)),
		PerIP:   ratelimit.New(10, 30),
		SignUps: ratelimit.New(ratelimit.Per(cfg.SignUpsPerMinute, time.Minute), max(cfg.SignUpsPerMinute, 1)),
		Logins:  ratelimit.New(ratelimit.Per(cfg.LoginsPerMinute, time.Minute), max(cfg.LoginsPerMinute, 1)),
	}
	posts := ratelimit.New(cfg.PostsPerSecond, int(2*cfg.PostsPerSecond)) // messages per user, over HTTP and gRPC alike

	svc := service.New(s).WithHub(h, pool.Submit).WithModeration(words.IsBanned).WithPostLimit(posts)

	// The browser app: the embedded web/dist, with "dist/" stripped so
	// index.html is at the root.
	app, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		return err
	}

	// HTTP. Listen first, so a port that's taken is reported straight away.
	gin.SetMode(gin.ReleaseMode) // no debug banner; requests are logged by api.RequestLog
	a := api.New(svc, limits).WithModeration(words, cfg.BannedWordsPath, cfg.AdminToken).WithWeb(app)
	httpLis, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler: a.Router(),
		// Without these, a client can hold a connection open forever by
		// sending its headers one byte at a time (a "slowloris" attack).
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10, // 16 KiB (the default is 1 MiB)
		// No WriteTimeout or ReadTimeout: both would cut off /stream, which is
		// meant to stay open. Body size is capped by api.LimitBody instead.
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
	go func() {
		logger.Info("HTTP listening", "url", "http://"+httpLis.Addr().String())
		if err := srv.Serve(httpLis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server stopped", "err", err)
			stop() // shut the rest down too
		}
	}()

	// gRPC: the same service behind a second front door.
	grpcLis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return err
	}
	gs := grpc.NewServer(
		// Log first, so calls the auth interceptor refuses are logged too.
		grpc.ChainUnaryInterceptor(grpcapi.UnaryLog(logger), grpcapi.UnaryAuth(svc, limits)),
		grpc.ChainStreamInterceptor(grpcapi.StreamLog(logger), grpcapi.StreamAuth(svc, limits)),
		// The default is 4 MiB per message; no request here needs more than a
		// few KB, so match the HTTP body cap.
		grpc.MaxRecvMsgSize(api.MaxBodyBytes),
		// At most this many calls at once on one connection (Watch, Converse
		// and Import each hold one open).
		grpc.MaxConcurrentStreams(100),
	)
	pb.RegisterChatServiceServer(gs, grpcapi.New(svc))
	reflection.Register(gs) // lets tools like grpcurl list the service without the .proto
	go func() {
		logger.Info("gRPC listening", "addr", grpcLis.Addr().String())
		if err := gs.Serve(grpcLis); err != nil {
			logger.Error("gRPC server stopped", "err", err)
			stop()
		}
	}()

	<-ctx.Done() // wait for Ctrl+C
	logger.Info("shutting down")

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
		logger.Warn("HTTP shutdown", "err", err)
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

	logger.Info("bye", "dropped_slow_clients", h.Dropped())
	return nil
}
