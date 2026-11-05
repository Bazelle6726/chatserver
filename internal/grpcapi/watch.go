package grpcapi

import (
	"chatserver/internal/chat"
	"chatserver/internal/pb"
	"chatserver/internal/pbconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Watch is server streaming: the gRPC version of GET /rooms/:id/stream.
// The call lasts until this method returns. Every stream.Send is one more
// message to the client.
func (s *Server) Watch(req *pb.WatchRequest, stream grpc.ServerStreamingServer[pb.Message]) error {

	room, err := s.store.GetRoom(chat.RoomID(req.GetRoomId()))
	if err != nil {
		return toStatus(err)
	}
	ctx := stream.Context()
	client := s.hub.Join(currentUser(ctx).ID, room.ID)
	defer s.hub.Leave(client)
	stream.SendHeader(metadata.Pairs("x-watching", string(room.ID)))
	for {
		select {
		case m, ok := <-client.Out():
			if !ok {
				return status.Error(codes.Unavailable, "hub dropped or shutting down")
			}
			if err := stream.Send(pbconv.Message(m)); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}
