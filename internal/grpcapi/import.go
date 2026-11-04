package grpcapi

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"chatserver/internal/pb"
)

// Import is client streaming: the client sends any number of messages, then
// closes its side, and gets one ImportSummary back.
//
// TODO(60):
//   - user := currentUser(stream.Context()), and a counter (int32)
//   - loop: req, err := stream.Recv()
//   - errors.Is(err, io.EOF): the client has finished sending. Reply and end
//     the call with stream.SendAndClose(&pb.ImportSummary{Posted: n})
//   - any other error: return it
//   - otherwise s.post(user.ID, req.GetRoomId(), req.GetText()); if that
//     fails, return toStatus(err) (this ends the whole import), else count it
//
// io.EOF here isn't a failure: it's how Recv says "nothing more is coming".
func (s *Server) Import(stream grpc.ClientStreamingServer[pb.PostMessageRequest, pb.ImportSummary]) error {
	return status.Error(codes.Unimplemented, "TODO(60): Import")
}
