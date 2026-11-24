package grpcapi

import (
	"errors"
	"io"

	"google.golang.org/grpc"

	"chatserver/internal/chat"
	"chatserver/internal/pb"
)

// Import is client streaming: the client sends any number of messages, then
// closes its side, and gets one ImportSummary back.
//
// io.EOF here isn't a failure: it's how Recv says "nothing more is coming".
func (s *Server) Import(stream grpc.ClientStreamingServer[pb.PostMessageRequest, pb.ImportSummary]) error {
	ctx := stream.Context()
	user := currentUser(ctx)
	var count int32
	for {
		req, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return stream.SendAndClose(&pb.ImportSummary{Posted: count})
			}
			return err
		}
		if _, err := s.svc.PostMessage(ctx, user.ID, chat.RoomID(req.GetRoomId()), req.GetText()); err != nil {
			return toStatus(err)
		}
		count++
	}
}
