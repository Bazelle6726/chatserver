package grpcapi

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"chatserver/internal/chat"
	"chatserver/internal/pb"
	"chatserver/internal/pbconv"
)

// Watch is server streaming: the gRPC version of GET /rooms/:id/stream.
// The call lasts until this method returns. Every stream.Send is one more
// message to the client.
func (s *Server) Watch(req *pb.WatchRequest, stream grpc.ServerStreamingServer[pb.Message]) error {
	ctx := stream.Context()
	client, leave, err := s.svc.Watch(ctx, currentUser(ctx).ID, chat.RoomID(req.GetRoomId())) // only members may watch
	if err != nil {
		return toStatus(err)
	}
	defer leave()
	if err := stream.SendHeader(metadata.Pairs("x-watching", req.GetRoomId())); err != nil {
		return err
	}
	for {
		select {
		case m, ok := <-client.Out():
			if !ok {
				return status.Error(codes.Unavailable, "stream closed by the server: too slow, or shutting down")
			}
			if err := stream.Send(pbconv.Message(m)); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}
