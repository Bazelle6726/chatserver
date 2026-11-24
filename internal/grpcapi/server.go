// Package grpcapi is the gRPC front door. It's the same job as package api,
// but for gRPC: turn calls into service calls and the results into protobuf
// messages. The chat rules all live in package service, shared with the
// HTTP API.
package grpcapi

import (
	"chatserver/internal/pb"
	"chatserver/internal/service"
)

// Server implements pb.ChatServiceServer, the interface protoc generated
// from the service block in chat.proto.
//
// Embedding pb.UnimplementedChatServiceServer gives Server a default version
// of every method that returns codes.Unimplemented. So the code compiles
// while methods are still missing, and adding a new rpc to chat.proto
// doesn't break the build. It must be embedded by value, not as a pointer.
type Server struct {
	pb.UnimplementedChatServiceServer

	svc *service.Service
}

// New builds the gRPC server on top of svc, the same service the HTTP API
// uses.
func New(svc *service.Service) *Server {
	return &Server{svc: svc}
}
