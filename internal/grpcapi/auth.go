package grpcapi

import (
	"context"
	"net"
	"strings"

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
// key-value pairs sent alongside it. Keys are always lowercase. The session
// token goes in "authorization", as "Bearer <token>", like the HTTP header.
const authKey = "authorization"

// Calls that don't need a session: signing up and logging in, like
// POST /users and POST /sessions in the HTTP API. The FullMethodName
// constants are generated from chat.proto.
var public = map[string]bool{
	pb.ChatService_CreateUser_FullMethodName: true,
	pb.ChatService_Login_FullMethodName:      true,
}

// userKey and tokenKey are the context keys for the logged-in user and their
// session token. Unexported types of our own mean no other package can ever
// use the same keys by accident.
type (
	userKey  struct{}
	tokenKey struct{}
)

// withUser returns a copy of ctx that also carries u and their token.
// Contexts are never changed in place: each WithValue wraps the old one.
func withUser(ctx context.Context, u *chat.User, token string) context.Context {
	return context.WithValue(context.WithValue(ctx, userKey{}, u), tokenKey{}, token)
}

// currentToken returns the session token the interceptor put on the context.
func currentToken(ctx context.Context) string {
	token, _ := ctx.Value(tokenKey{}).(string)
	return token
}

// currentUser returns the user the interceptor put on the context.
func currentUser(ctx context.Context) *chat.User {
	u, _ := ctx.Value(userKey{}).(*chat.User)
	return u
}

// authenticate finds the calling user from the session token in the call's
// metadata, and returns the user and the token.
func authenticate(ctx context.Context, svc *service.Service) (*chat.User, string, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	vals := md.Get(authKey)
	if len(vals) == 0 {
		return nil, "", status.Error(codes.Unauthenticated, "missing authorization metadata (Bearer <token>)")
	}
	token, ok := strings.CutPrefix(vals[0], "Bearer ")
	if !ok {
		return nil, "", status.Error(codes.Unauthenticated, "authorization must be \"Bearer <token>\"")
	}
	u, err := svc.Authenticate(ctx, strings.TrimSpace(token))
	if err != nil {
		return nil, "", toStatus(err)
	}
	return u, token, nil
}

// UnaryAuth is gRPC's version of the Auth and rate-limit middleware, for
// unary calls. An interceptor wraps every call, like Gin middleware: calling
// handler is c.Next(), and returning an error instead is c.Abort().
//
// Public calls are limited per client IP (and CreateUser and Login by the
// sign-up and login limits too); every other call needs a user and is limited per user, with
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
			if info.FullMethod == pb.ChatService_Login_FullMethodName && !limits.Logins.Allow(ip) {
				return nil, toStatus(ratelimit.ErrLimited)
			}
			return handler(ctx, req)
		}
		u, token, err := authenticate(ctx, svc)
		if err != nil {
			return nil, err
		}
		if !limits.PerUser.Allow(string(u.ID)) {
			return nil, toStatus(ratelimit.ErrLimited)
		}
		return handler(withUser(ctx, u, token), req)
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
		u, token, err := authenticate(ss.Context(), svc)
		if err != nil {
			return err
		}
		if !limits.PerUser.Allow(string(u.ID)) {
			return toStatus(ratelimit.ErrLimited)
		}
		return handler(srv, &authedStream{ServerStream: ss, ctx: withUser(ss.Context(), u, token)})
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
