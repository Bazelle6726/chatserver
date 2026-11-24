package grpcapi

import (
	"context"

	"chatserver/internal/chat"
	"chatserver/internal/pb"
	"chatserver/internal/pbconv"
)

// Each unary method is a handler like in package api, but simpler: gRPC has
// already decoded the request into a struct, and the reply is just a return
// value. The generated getters (req.GetName()) are safe even on a nil
// request. On failure, return nil and toStatus(err).

// CreateUser signs up and logs in: the reply carries a session token.
func (s *Server) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.AuthResponse, error) {
	u, token, err := s.svc.Register(ctx, req.GetName(), req.GetPassword())
	if err != nil {
		return nil, toStatus(err)
	}
	return &pb.AuthResponse{User: pbconv.User(u), Token: token}, nil
}

func (s *Server) Login(ctx context.Context, req *pb.LoginRequest) (*pb.AuthResponse, error) {
	u, token, err := s.svc.Login(ctx, req.GetName(), req.GetPassword())
	if err != nil {
		return nil, toStatus(err)
	}
	return &pb.AuthResponse{User: pbconv.User(u), Token: token}, nil
}

// Logout ends the session this call was made with.
func (s *Server) Logout(ctx context.Context, req *pb.LogoutRequest) (*pb.LogoutResponse, error) {
	s.svc.Logout(currentToken(ctx))
	return &pb.LogoutResponse{}, nil
}

func (s *Server) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.User, error) {
	u, err := s.svc.GetUser(ctx, chat.UserID(req.GetId()))
	if err != nil {
		return nil, toStatus(err)
	}
	return pbconv.User(u), nil
}

func (s *Server) CreateRoom(ctx context.Context, req *pb.CreateRoomRequest) (*pb.Room, error) {
	r, err := s.svc.CreateRoom(ctx, req.GetName(), int(req.GetMaxHistory()))
	if err != nil {
		return nil, toStatus(err)
	}
	return pbconv.Room(r), nil
}

func (s *Server) ListRooms(ctx context.Context, req *pb.ListRoomsRequest) (*pb.ListRoomsResponse, error) {
	rooms, err := s.svc.ListRooms(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	pbRooms := make([]*pb.Room, len(rooms))
	for i, r := range rooms {
		pbRooms[i] = pbconv.Room(r)
	}
	return &pb.ListRoomsResponse{Rooms: pbRooms}, nil
}

func (s *Server) JoinRoom(ctx context.Context, req *pb.JoinRoomRequest) (*pb.JoinRoomResponse, error) {
	if err := s.svc.JoinRoom(ctx, currentUser(ctx).ID, chat.RoomID(req.GetRoomId())); err != nil {
		return nil, toStatus(err)
	}
	return &pb.JoinRoomResponse{}, nil
}

func (s *Server) PostMessage(ctx context.Context, req *pb.PostMessageRequest) (*pb.Message, error) {
	m, err := s.svc.PostMessage(ctx, currentUser(ctx).ID, chat.RoomID(req.GetRoomId()), req.GetText())
	if err != nil {
		return nil, toStatus(err)
	}
	return pbconv.Message(m), nil
}

// History: limit 0 (unset) means the default.
func (s *Server) History(ctx context.Context, req *pb.HistoryRequest) (*pb.HistoryResponse, error) {
	ms, err := s.svc.History(ctx, currentUser(ctx).ID, chat.RoomID(req.GetRoomId()), int(req.GetLimit()))
	if err != nil {
		return nil, toStatus(err)
	}
	return &pb.HistoryResponse{Messages: pbconv.Messages(ms)}, nil
}
