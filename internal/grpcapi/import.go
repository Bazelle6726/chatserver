package grpcapi

import (
	"chatserver/internal/pb"
	"errors"
	"io"

	"google.golang.org/grpc"
)

// Import is client streaming: the client sends any number of messages, then
// closes its side, and gets one ImportSummary back.
//
// io.EOF here isn't a failure: it's how Recv says "nothing more is coming".
func (s *Server) Import(stream grpc.ClientStreamingServer[pb.PostMessageRequest, pb.ImportSummary]) error {
	user := currentUser(stream.Context())
	var count int32
	for {
		req, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return stream.SendAndClose(&pb.ImportSummary{Posted: count})
			}
			return err
		}
		_, err = s.post(user.ID, req.GetRoomId(), req.GetText())
		if err != nil {
			return toStatus(err)
		}
		count++
	}
	//return status.Error(codes.Unimplemented, "TODO(60): Import")
}
