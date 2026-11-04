package grpcapi

import (
	"errors"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"chatserver/internal/chat"
	"chatserver/internal/pb"
)

// Converse is bidirectional streaming: one call where the client posts
// messages and receives the room's messages, at the same time.
//
// gRPC allows one goroutine calling Send and another calling Recv on the same
// stream at once, but not two of either. So:
//   - a new goroutine is the only one calling stream.Recv (and posting)
//   - this goroutine is the only one calling stream.Send
func (s *Server) Converse(stream grpc.BidiStreamingServer[pb.ConverseRequest, pb.Message]) error {
	if s.hub == nil {
		return status.Error(codes.Unavailable, "live delivery is off")
	}
	ctx := stream.Context()
	user := currentUser(ctx)

	// The first message must say which room. GetJoinRoomId returns "" if the
	// oneof holds the other field (text) instead.
	first, err := stream.Recv()
	if errors.Is(err, io.EOF) {
		return nil // the client closed before saying anything
	}
	if err != nil {
		return err
	}
	roomID := first.GetJoinRoomId()
	if roomID == "" {
		return status.Error(codes.InvalidArgument, "the first message must set join_room_id")
	}
	room, err := s.store.GetRoom(chat.RoomID(roomID))
	if err != nil {
		return toStatus(err)
	}
	if !room.IsMember(user.ID) {
		if err := room.Join(user.ID); err != nil {
			return toStatus(err)
		}
	}
	client := s.hub.Join(user.ID, room.ID)
	defer s.hub.Leave(client)
	if err := stream.SendHeader(metadata.Pairs("x-watching", roomID)); err != nil {
		return err
	}

	// TODO(61): the reader goroutine.
	//   - recvErr := make(chan error, 1). Buffered, so the goroutine can always
	//     hand over its error and exit, even if Converse has already returned
	//     (otherwise it would be stuck forever: a goroutine leak)
	//   - go func() { for { ... } }(): req, err := stream.Recv(); on error send
	//     it on recvErr and return. Otherwise s.post(user.ID, roomID, req.GetText());
	//     if that fails, send toStatus(err) on recvErr and return.

	// TODO(62): the writer loop, this goroutine. Like Watch, plus one case:
	//   - m, ok := <-client.Out(): !ok -> return codes.Unavailable;
	//     else stream.Send(toPBMessage(m)), returning its error
	//   - err := <-recvErr: io.EOF means the client finished: return nil.
	//     Anything else: return err
	//   - <-ctx.Done(): return nil
	return status.Error(codes.Unimplemented, "TODO(61-62): Converse")
}
