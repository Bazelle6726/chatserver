package api

import (
	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
)

// Stage 5: content negotiation. The same routes speak JSON or protobuf.
//
//	request body:  Content-Type says what the client SENT
//	response body: Accept says what the client WANTS back
//
// The two are independent: a client can send JSON and ask for protobuf back.
// JSON stays the default, so every existing client keeps working.
//
// Until you finish these, they behave like stage 4: JSON only.
const (
	mimeProto    = "application/x-protobuf" // what Gin and most tools use
	mimeProtoAlt = "application/protobuf"   // the newer, registered name
)

// isProto reports whether a media type is one of the protobuf ones.
func isProto(mediaType string) bool {
	return mediaType == mimeProto || mediaType == mimeProtoAlt
}

// wantsProto reports whether the client asked for a protobuf response.
//
// TODO(64): read the Accept header with c.GetHeader("Accept"). It can list
// several types, e.g. "application/x-protobuf, application/json;q=0.5", so
// don't compare it with ==. Return true if it contains mimeProto or
// mimeProtoAlt anywhere (strings.Contains; add the "strings" import).
func wantsProto(c *gin.Context) bool {
	return false
}

// respond sends a success response in the format the client asked for:
// dto as JSON (the default), or msg as protobuf.
//
// TODO(65):
//   - !wantsProto(c): c.JSON(code, dto) and return, exactly as before
//   - otherwise data, err := proto.Marshal(msg). On error, writeError(c, err)
//     (a 500: a message that won't marshal is our bug, not the client's)
//   - else c.Data(code, mimeProto, data): raw bytes with that Content-Type
//
// Errors always go out as JSON through writeError, whatever Accept says.
// (A protobuf client still has to understand JSON errors: a trade-off to
// mention.)
func respond(c *gin.Context, code int, dto any, msg proto.Message) {
	c.JSON(code, dto)
}

// bindBody reads the request body in whichever format the client sent.
//
//   - JSON (the default) goes straight into req, with its binding tags
//   - protobuf is decoded into msg, then fill copies its fields into req
//
// Either way the handler ends up with req filled in and carries on the same.
//
// TODO(66):
//   - !isProto(c.ContentType()): keep what's here (ShouldBindJSON)
//   - otherwise data, err := c.GetRawData() (the body as []byte)
//   - proto.Unmarshal(data, msg)
//   - then call fill()
//
// Any failure: return invalid("body", ...) with the error's text, so it's a
// 400 on the field "body", the same as bad JSON.
func bindBody(c *gin.Context, req any, msg proto.Message, fill func()) error {
	if err := c.ShouldBindJSON(req); err != nil {
		return invalid("body", err.Error())
	}
	return nil
}
