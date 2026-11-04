package grpcapi

import (
	"context"

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

// like createRoom over HTTP:
func (s *Server) CreateRoom(ctx context.Context, req *pb.CreateRoomRequest) (*pb.Room, error) {
	max := int(req.GetMaxHistory())
	if max == 0 {
		max = defaultMaxHistory
	}
	if max < 1 || max > maxMaxHistory {
		return nil, toStatus(invalid("max_history", "must be 1-1000"))
	}
	r, err := s.store.CreateRoom(req.GetName(), max)
	if err != nil {
		return nil, toStatus(err)
	}
	return toPBRoom(r), nil
	//return nil, status.Error(codes.Unimplemented, "TODO(54): CreateRoom")
}

// Converts every room from s.store.ListRooms() into a []*pb.Room
// and returns &pb.ListRoomsResponse{Rooms: ...}
func (s *Server) ListRooms(ctx context.Context, req *pb.ListRoomsRequest) (*pb.ListRoomsResponse, error) {
	List := s.store.ListRooms()
	pbRooms := make([]*pb.Room, len(List))
	for i, r := range List {
		pbRooms[i] = toPBRoom(r)
	}
	return &pb.ListRoomsResponse{Rooms: pbRooms}, nil
	//return nil, status.Error(codes.Unimplemented, "TODO(55): ListRooms")
}

// get the room, then room.Join(currentUser(ctx).ID).
func (s *Server) JoinRoom(ctx context.Context, req *pb.JoinRoomRequest) (*pb.JoinRoomResponse, error) {
	user := currentUser(ctx)
	room, err := s.store.GetRoom(chat.RoomID(req.GetRoomId())) //get the room
	if err != nil {
		return nil, toStatus(err)
	}
	err = room.Join(user.ID) //join the room with the current user's ID
	if err != nil {
		return nil, toStatus(err)
	}
	return &pb.JoinRoomResponse{}, nil //return &pb.JoinRoomResponse{}, nil
	//return nil, status.Error(codes.Unimplemented, "TODO(56): JoinRoom")
}

// Uses s.post (from server.go): stores the message AND publishes
// it. Return toPBMessage of the stored message.
func (s *Server) PostMessage(ctx context.Context, req *pb.PostMessageRequest) (*pb.Message, error) {
	user := currentUser(ctx)
	msg, err := s.post(user.ID, req.GetRoomId(), req.GetText())
	if err != nil {
		return nil, toStatus(err)
	}
	return toPBMessage(msg), nil
	//return nil, status.Error(codes.Unimplemented, "TODO(57): PostMessage")
}

func (s *Server) History(ctx context.Context, req *pb.HistoryRequest) (*pb.HistoryResponse, error) {
	room, err := s.store.GetRoom(chat.RoomID(req.GetRoomId())) //get the room
	if err != nil {
		return nil, toStatus(err)
	}
	limit := int(req.GetLimit()) //get the limit
	if limit == 0 {
		limit = defaultLimit
	}
	if limit < 1 || limit > maxLimit {
		return nil, toStatus(invalid("limit", "must be 1-500"))
	}
	return &pb.HistoryResponse{Messages: toPBMessages(room.LastN(limit))}, nil
}
