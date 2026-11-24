package grpcapi

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UnaryLog and StreamLog log one structured line per call with log/slog,
// like the HTTP API's RequestLog:
//
//	level=INFO msg="grpc call" method=/chat.v1.ChatService/PostMessage code=OK ms=0.3 ip=127.0.0.1
//
// Chain them before the auth interceptors, so refused calls are logged too.
// Calls that fail with Internal or Unknown are logged at Error level.
func UnaryLog(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		logCall(ctx, logger, info.FullMethod, start, err)
		return resp, err
	}
}

// StreamLog is UnaryLog for streaming calls. The line is written when the
// stream ends, so ms is how long it was open.
func StreamLog(logger *slog.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, ss)
		logCall(ss.Context(), logger, info.FullMethod, start, err)
		return err
	}
}

func logCall(ctx context.Context, logger *slog.Logger, method string, start time.Time, err error) {
	code := status.Code(err)
	level := slog.LevelInfo
	if code == codes.Internal || code == codes.Unknown {
		level = slog.LevelError
	}
	logger.LogAttrs(ctx, level, "grpc call",
		slog.String("method", method),
		slog.String("code", code.String()),
		slog.Float64("ms", float64(time.Since(start).Microseconds())/1000),
		slog.String("ip", clientIP(ctx)),
	)
}
