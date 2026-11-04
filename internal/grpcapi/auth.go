package grpcapi

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"chatserver/internal/chat"
	"chatserver/internal/pb"
	"chatserver/internal/store"
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
//
// TODO(51):
//   - md, _ := metadata.FromIncomingContext(ctx)   (import google.golang.org/grpc/metadata)
//   - md.Get(userIDKey) returns a []string (a key can have several values)
//   - none, or the first one is "": return codes.Unauthenticated,
//     "missing x-user-id metadata"
//   - s.GetUser(...) fails: return codes.Unauthenticated, "unknown user"
//     (not NotFound: to the caller it means "you're not logged in")
//   - otherwise return the user
//
// md is nil when there's no metadata at all, and Get on a nil map is fine.
func authenticate(ctx context.Context, s store.Store) (*chat.User, error) {
	return nil, status.Error(codes.Unimplemented, "TODO(51): authenticate")
}

// UnaryAuth is gRPC's version of the Auth middleware, for unary calls.
// An interceptor wraps every call, like Gin middleware: calling handler is
// c.Next(), and returning an error instead is c.Abort().
//
// TODO(52): for a non-public method, authenticate; on error return nil, err.
// Otherwise call the handler with a context that carries the user:
// handler(withUser(ctx, u), req)
func UnaryAuth(s store.Store) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if public[info.FullMethod] {
			return handler(ctx, req)
		}
		return nil, status.Error(codes.Unimplemented, "TODO(52): UnaryAuth")
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
//
// TODO(53): authenticate with ss.Context(); on error return it.
// Otherwise call the handler with a wrapped stream:
//
//	handler(srv, &authedStream{ServerStream: ss, ctx: withUser(ss.Context(), u)})
func StreamAuth(s store.Store) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return status.Error(codes.Unimplemented, "TODO(53): StreamAuth")
	}
}
