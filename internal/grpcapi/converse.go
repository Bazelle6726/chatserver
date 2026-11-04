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

	recvErr := make(chan error, 1)
	go func() {
		for {
			req, err := stream.Recv()
			if err != nil {
				recvErr <- err // includes io.EOF: the client closed its side
				return
			}
			if _, err := s.post(user.ID, roomID, req.GetText()); err != nil {
				recvErr <- toStatus(err)
				return
			}
		}
	}()

	for {
		select {
		case m, ok := <-client.Out():
			if !ok {
				return status.Error(codes.Unavailable, "stream closed by the server: too slow, or shutting down")
			}
			if err := stream.Send(toPBMessage(m)); err != nil {
				return err
			}
		case err := <-recvErr:
			if errors.Is(err, io.EOF) {
				return nil // the client said it's done
			}
			return err
		case <-ctx.Done():
			return nil // the client went away
		}
	}
}
