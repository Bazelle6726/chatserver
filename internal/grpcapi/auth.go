package grpcapi

import (
	"context"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"chatserver/internal/chat"
	"chatserver/internal/pb"
	"chatserver/internal/ratelimit"
	"chatserver/internal/service"
)

// gRPC has no headers in the HTTP sense, but every call carries metadata:
// key-value pairs sent alongside it. Keys are always lowercase.
const userIDKey = "x-user-id"

// Calls that don't need a logged-in user, like POST /users in the HTTP API.
// The FullMethodName constants are generated from chat.proto.
var public = map[string]bool{
	pb.ChatService_CreateUser_FullMethodName: true,
	pb.ChatService_GetUser_FullMethodName:    true,
}

// userKey is the context key for the logged-in user. An unexported type of
// our own means no other package can ever use the same key by accident.
type userKey struct{}

// withUser returns a copy of ctx that also carries u. Contexts are never
// changed in place: each WithValue wraps the old one.
func withUser(ctx context.Context, u *chat.User) context.Context {
	return context.WithValue(ctx, userKey{}, u)
}

// currentUser returns the user the interceptor put on the context.
func currentUser(ctx context.Context) *chat.User {
	u, _ := ctx.Value(userKey{}).(*chat.User)
	return u
}

// authenticate finds the calling user from the call's metadata.
func authenticate(ctx context.Context, svc *service.Service) (*chat.User, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	userID := md.Get(userIDKey)
	if len(userID) == 0 || userID[0] == "" {
		return nil, status.Error(codes.Unauthenticated, "missing x-user-id metadata")
	}
	u, err := svc.GetUser(ctx, chat.UserID(userID[0]))
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "unknown user")
	}
	return u, nil
}

// UnaryAuth is gRPC's version of the Auth and rate-limit middleware, for
// unary calls. An interceptor wraps every call, like Gin middleware: calling
// handler is c.Next(), and returning an error instead is c.Abort().
//
// Public calls are limited per client IP (and CreateUser by the sign-up
// limit too); every other call needs a user and is limited per user, with
// the same limits as the HTTP API.
func UnaryAuth(svc *service.Service, limits ratelimit.Limits) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if public[info.FullMethod] {
			ip := clientIP(ctx)
			if !limits.PerIP.Allow(ip) {
				return nil, toStatus(ratelimit.ErrLimited)
			}
			if info.FullMethod == pb.ChatService_CreateUser_FullMethodName && !limits.SignUps.Allow(ip) {
				return nil, toStatus(ratelimit.ErrLimited)
			}
			return handler(ctx, req)
		}
		u, err := authenticate(ctx, svc)
		if err != nil {
			return nil, err
		}
		if !limits.PerUser.Allow(string(u.ID)) {
			return nil, toStatus(ratelimit.ErrLimited)
		}
		return handler(withUser(ctx, u), req)
	}
}

// authedStream wraps a stream so that Context() returns our context, the one
// that carries the user. Everything else comes from the embedded stream.
//
// A stream's context can't be replaced, so this is how you add a value to
// it: embed the original and override the one method. Embedding promotes
// all of ServerStream's methods, so *authedStream is a grpc.ServerStream too.
type authedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (a *authedStream) Context() context.Context { return a.ctx }

// StreamAuth is the same for streaming calls. Every streaming call needs a
// logged-in user, and opening one counts as one request. Messages sent
// inside a stream (Import, Converse) count against the service's per-user
// posting limit instead.
func StreamAuth(svc *service.Service, limits ratelimit.Limits) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		u, err := authenticate(ss.Context(), svc)
		if err != nil {
			return err
		}
		if !limits.PerUser.Allow(string(u.ID)) {
			return toStatus(ratelimit.ErrLimited)
		}
		return handler(srv, &authedStream{ServerStream: ss, ctx: withUser(ss.Context(), u)})
	}
}

// clientIP is the address of the client that made the call, without its
// port, for the per-IP limits. "" if it can't be told.
func clientIP(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok || p.Addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(p.Addr.String())
	if err != nil {
		return p.Addr.String() // e.g. a bufconn address in tests
	}
	return host
}
