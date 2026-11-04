// Package grpcapi is the gRPC front door. It's the same job as package api,
// but for gRPC: turn calls into Store and Hub operations and turn the results
// into protobuf messages. All the chat logic stays in chat, store and hub.
package grpcapi

import (
	"chatserver/internal/chat"
	"chatserver/internal/hub"
	"chatserver/internal/pb"
	"chatserver/internal/store"
)

const (
	defaultMaxHistory = 100
	maxMaxHistory     = 1000
	defaultLimit      = 50
	maxLimit          = 500
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

	store   store.Store
	hub     *hub.Hub             // nil = no live delivery (Watch and Converse refuse)
	publish func(m chat.Message) // nil = don't publish (e.g. in some tests)
}

// New builds the gRPC server. In main, publish is the worker pool's Submit,
// exactly as for the HTTP API.
func New(s store.Store, h *hub.Hub, publish func(chat.Message)) *Server {
	return &Server{store: s, hub: h, publish: publish}
}

// post stores a message and hands it to the live feed. PostMessage, Import and
// Converse all post messages, so they share this.
func (s *Server) post(author chat.UserID, room string, text string) (chat.Message, error) {
	m, err := s.store.PostMessage(chat.RoomID(room), author, text)
	if err != nil {
		return chat.Message{}, err
	}
	if s.publish != nil {
		s.publish(m)
	}
	return m, nil
}
