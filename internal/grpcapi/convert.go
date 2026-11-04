package grpcapi

import (
	"chatserver/internal/chat"
	"chatserver/internal/pb"
)

// These convert domain types into the generated protobuf types. They're the
// gRPC version of api/dto.go.
func toPBRole(r chat.Role) pb.Role {
	switch r {
	case chat.RoleGuest:
		return pb.Role_ROLE_GUEST
	case chat.RoleMember:
		return pb.Role_ROLE_MEMBER
	case chat.RoleAdmin:
		return pb.Role_ROLE_ADMIN
	default:
		return pb.Role_ROLE_UNSPECIFIED
	}
}

// toPBUser: &pb.User{...} with Id, Name and Role (use toPBRole).
// IDs are defined types: converted with string(u.ID).
func toPBUser(u *chat.User) *pb.User {
	return &pb.User{
		Id:   string(u.ID),
		Name: u.Name,
		Role: toPBRole(u.Role),
	}
}

// toPBRoom: Id, Name, and Members from r.MemberCount(). Members is an int32
// in proto, so converted: int32(...).
func toPBRoom(r *chat.Room) *pb.Room {
	return &pb.Room{
		Id:      string(r.ID),
		Name:    r.Name,
		Members: int32(r.MemberCount()),
	}
}

// toPBMessage: Id, RoomId, AuthorId, Kind, Text, CreatedAtUnixMilli.
func toPBMessage(m chat.Message) *pb.Message {
	return &pb.Message{
		Id:                 string(m.ID),
		RoomId:             string(m.Room),
		AuthorId:           string(m.Author),
		Kind:               pb.MessageKind(m.Kind),
		Text:               m.Text,
		CreatedAtUnixMilli: m.CreatedAt.UnixMilli(),
	}
}

// toPBMessages converts a slice. Return an empty slice, not nil, for no messages.
func toPBMessages(ms []chat.Message) []*pb.Message {
	pbMessages := make([]*pb.Message, len(ms))
	for i, m := range ms {
		pbMessages[i] = toPBMessage(m)
	}
	return pbMessages
}
