package grpcapi

import (
	"chatserver/internal/chat"
	"chatserver/internal/pb"
)

// These convert domain types into the generated protobuf types. They're the
// gRPC version of api/dto.go.
//
// Open internal/pb/chat.pb.go (generated) to see the Go names protoc chose:
// room_id became RoomId, the enum value ROLE_GUEST became pb.Role_ROLE_GUEST.
//
// Generated messages are always used through pointers (*pb.User), never
// copied: each one contains internal state that mustn't be copied.

// TODO(49): write all five functions.
//
// toPBRole: a switch. The numbers DON'T line up (chat.RoleGuest is 0, but in
// proto 0 is ROLE_UNSPECIFIED and ROLE_GUEST is 1), so map each value by name:
// RoleGuest -> pb.Role_ROLE_GUEST, and so on; anything else -> ROLE_UNSPECIFIED.
func toPBRole(r chat.Role) pb.Role {
	return pb.Role_ROLE_UNSPECIFIED
}

// toPBUser: &pb.User{...} with Id, Name and Role (use toPBRole).
// IDs are defined types: convert with string(u.ID).
func toPBUser(u *chat.User) *pb.User {
	return &pb.User{}
}

// toPBRoom: Id, Name, and Members from r.MemberCount(). Members is an int32
// in proto, so convert: int32(...).
func toPBRoom(r *chat.Room) *pb.Room {
	return &pb.Room{}
}

// toPBMessage: Id, RoomId, AuthorId, Kind, Text, CreatedAtUnixMilli.
//   - here the enum numbers DO line up on purpose, so a conversion is enough:
//     pb.MessageKind(m.Kind)
//   - m.CreatedAt is a time.Time: use its UnixMilli() method
func toPBMessage(m chat.Message) *pb.Message {
	return &pb.Message{}
}

// toPBMessages converts a slice. Return an empty slice, not nil, for no
// messages: make([]*pb.Message, len(ms)), then fill it in a loop.
func toPBMessages(ms []chat.Message) []*pb.Message {
	return nil
}
