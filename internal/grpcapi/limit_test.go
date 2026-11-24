package grpcapi

import (
	"context"
	"net"
	"testing"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"

	"chatserver/internal/pb"
	"chatserver/internal/ratelimit"
	"chatserver/internal/service"
	"chatserver/internal/store"
)

// fromIP is a context as the server sees a call from ip.
func fromIP(ip string) context.Context {
	return peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP(ip), Port: 40000}})
}

func ok(context.Context, any) (any, error) { return "ok", nil }

// The gRPC API applies the same limits as the HTTP API: sign-ups per IP,
// and every call per user.
func TestInterceptorLimits(t *testing.T) {
	svc := service.New(store.NewMemoryStore()).WithPasswordCost(bcrypt.MinCost)
	limits := ratelimit.Limits{
		SignUps: ratelimit.New(1, 1),
		PerUser: ratelimit.New(1, 2),
	}
	intercept := UnaryAuth(svc, limits)
	signUp := &grpc.UnaryServerInfo{FullMethod: pb.ChatService_CreateUser_FullMethodName}
	listRooms := &grpc.UnaryServerInfo{FullMethod: pb.ChatService_ListRooms_FullMethodName}

	if _, err := intercept(fromIP("10.0.0.1"), nil, signUp, ok); err != nil {
		t.Fatalf("first sign-up: %v", err)
	}
	_, err := intercept(fromIP("10.0.0.1"), nil, signUp, ok)
	wantCode(t, err, codes.ResourceExhausted, "2nd sign-up from the same IP")
	if _, err := intercept(fromIP("10.0.0.2"), nil, signUp, ok); err != nil {
		t.Errorf("sign-up from another IP: %v", err)
	}

	_, token, err := svc.Register(t.Context(), "alice", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	asAlice := metadata.NewIncomingContext(fromIP("10.0.0.1"), metadata.Pairs("authorization", "Bearer "+token))
	for i := range 2 {
		if _, err := intercept(asAlice, nil, listRooms, ok); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}
	_, err = intercept(asAlice, nil, listRooms, ok)
	wantCode(t, err, codes.ResourceExhausted, "3rd call in a burst of 2")
}
