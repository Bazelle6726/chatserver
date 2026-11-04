package grpcapi

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"chatserver/internal/pb"
)

// Watch is server streaming: the gRPC version of GET /rooms/:id/stream.
// The call lasts until this method returns. Every stream.Send is one more
// message to the client.
//
// TODO(59): follow api/stream.go, step by step:
//  1. s.hub == nil: return codes.Unavailable, "live delivery is off"
//  2. get the room (chat.RoomID(req.GetRoomId())); on error return toStatus(err)
//  3. ctx := stream.Context(); join the hub with currentUser(ctx).ID and
//     room.ID, and defer s.hub.Leave(client)
//  4. tell the client we're watching (the test waits for this, like SSE's
//     ": connected" line), with metadata from google.golang.org/grpc/metadata:
//     stream.SendHeader(metadata.Pairs("x-watching", string(room.ID)))
//  5. loop with select:
//     - m, ok := <-client.Out(): if !ok return codes.Unavailable (dropped or
//     shutting down); else stream.Send(toPBMessage(m)), returning its error
//     - <-ctx.Done(): return nil (the client went away)
func (s *Server) Watch(req *pb.WatchRequest, stream grpc.ServerStreamingServer[pb.Message]) error {
	return status.Error(codes.Unimplemented, "TODO(59): Watch")
}
