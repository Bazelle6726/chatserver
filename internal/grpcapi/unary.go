package grpcapi

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"chatserver/internal/chat"
	"chatserver/internal/pb"
)

// Each unary method is a handler like in package api, but simpler: gRPC has
// already decoded the request into a struct, and the reply is just a return
// value. Use the generated getters (req.GetName()), which are safe even on a
// nil request. On failure return nil and toStatus(err).
//
// CreateUser and GetUser are written for you as examples.

func (s *Server) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.User, error) {
	u, err := s.store.CreateUser(req.GetName())
	if err != nil {
		return nil, toStatus(err)
	}
	return toPBUser(u), nil
}

func (s *Server) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.User, error) {
	u, err := s.store.GetUser(chat.UserID(req.GetId()))
	if err != nil {
		return nil, toStatus(err)
	}
	return toPBUser(u), nil
}

// TODO(54): like createRoom over HTTP:
//   - max := int(req.GetMaxHistory()); 0 means defaultMaxHistory
//   - below 0 or above maxMaxHistory: toStatus(invalid("max_history", "must be 1-1000"))
//   - s.store.CreateRoom, then return toPBRoom(r)
func (s *Server) CreateRoom(ctx context.Context, req *pb.CreateRoomRequest) (*pb.Room, error) {
	return nil, status.Error(codes.Unimplemented, "TODO(54): CreateRoom")
}

// TODO(55): convert every room from s.store.ListRooms() into a []*pb.Room
// and return &pb.ListRoomsResponse{Rooms: ...}
func (s *Server) ListRooms(ctx context.Context, req *pb.ListRoomsRequest) (*pb.ListRoomsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "TODO(55): ListRooms")
}

// TODO(56): get the room, then room.Join(currentUser(ctx).ID).
// Return &pb.JoinRoomResponse{} (empty, but never nil) on success.
func (s *Server) JoinRoom(ctx context.Context, req *pb.JoinRoomRequest) (*pb.JoinRoomResponse, error) {
	return nil, status.Error(codes.Unimplemented, "TODO(56): JoinRoom")
}

// TODO(57): use s.post (in server.go): it stores the message AND publishes
// it. Return toPBMessage of the stored message.
func (s *Server) PostMessage(ctx context.Context, req *pb.PostMessageRequest) (*pb.Message, error) {
	return nil, status.Error(codes.Unimplemented, "TODO(57): PostMessage")
}

// TODO(58): like history over HTTP:
//   - get the room
//   - limit := int(req.GetLimit()); 0 means defaultLimit
//   - below 1 or above maxLimit: toStatus(invalid("limit", "must be 1-500"))
//   - return &pb.HistoryResponse{Messages: toPBMessages(room.LastN(limit))}
func (s *Server) History(ctx context.Context, req *pb.HistoryRequest) (*pb.HistoryResponse, error) {
	return nil, status.Error(codes.Unimplemented, "TODO(58): History")
}
