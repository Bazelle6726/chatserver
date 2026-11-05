package pbconv

import (
	"chatserver/internal/chat"
	"chatserver/internal/pb"
)

func role(r chat.Role) pb.Role {
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

// User converts a user.
func User(u *chat.User) *pb.User {
	return &pb.User{
		Id:   string(u.ID),
		Name: u.Name,
		Role: role(u.Role),
	}
}

// Room converts a room.
func Room(r *chat.Room) *pb.Room {
	return &pb.Room{
		Id:      string(r.ID),
		Name:    r.Name,
		Members: int32(r.MemberCount()),
	}
}

// Message converts a message.
func Message(m chat.Message) *pb.Message {
	return &pb.Message{
		Id:                 string(m.ID),
		RoomId:             string(m.Room),
		AuthorId:           string(m.Author),
		Kind:               pb.MessageKind(m.Kind),
		Text:               m.Text,
		CreatedAtUnixMilli: m.CreatedAt.UnixMilli(),
	}
}

// Messages converts a slice of messages (never returns nil).
func Messages(ms []chat.Message) []*pb.Message {
	pbMessages := make([]*pb.Message, len(ms))
	for i, m := range ms {
		pbMessages[i] = Message(m)
	}
	return pbMessages
}
