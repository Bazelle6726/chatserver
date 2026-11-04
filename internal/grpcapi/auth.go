package grpcapi

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"chatserver/internal/chat"
	"chatserver/internal/pb"
	"chatserver/internal/store"

	"google.golang.org/grpc/metadata"
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
func authenticate(ctx context.Context, s store.Store) (*chat.User, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	userID := md.Get(userIDKey)
	if len(userID) == 0 || userID[0] == "" {
		return nil, status.Error(codes.Unauthenticated, "missing x-user-id metadata")
	}
	u, err := s.GetUser(chat.UserID(userID[0]))
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "unknown user")
	}
	return u, nil
}

// UnaryAuth is gRPC's version of the Auth middleware, for unary calls.
// An interceptor wraps every call, like Gin middleware: calling handler is
// c.Next(), and returning an error instead is c.Abort().
func UnaryAuth(s store.Store) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if public[info.FullMethod] {
			return handler(ctx, req)
		}
		u, err := authenticate(ctx, s)
		if err != nil {
			return nil, err
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
// logged-in user.
func StreamAuth(s store.Store) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		u, err := authenticate(ss.Context(), s)
		if err != nil {
			return err
		}
		return handler(srv, &authedStream{ServerStream: ss, ctx: withUser(ss.Context(), u)})
	}
}
