package api

import (
	"strings"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
)

// Stage 5: content negotiation. The same routes speak JSON or protobuf.
const (
	mimeProto    = "application/x-protobuf" // what Gin and most tools use
	mimeProtoAlt = "application/protobuf"   // the newer, registered name
)

// isProto reports whether a media type is one of the protobuf ones.
func isProto(mediaType string) bool {
	return mediaType == mimeProto || mediaType == mimeProtoAlt
}

// wantsProto reports whether the client asked for a protobuf response.
func wantsProto(c *gin.Context) bool {
	accept := c.GetHeader("Accept")
	if strings.Contains(accept, mimeProto) || strings.Contains(accept, mimeProtoAlt) {
		return true
	}
	return false
}

// respond sends a success response in the format the client asked for:
// dto as JSON (the default), or msg as protobuf.
func respond(c *gin.Context, code int, dto any, msg proto.Message) {
	if wantsProto(c) {
		data, err := proto.Marshal(msg)
		if err != nil {
			writeError(c, err)
			return
		}
		c.Data(code, mimeProto, data)
	} else {
		c.JSON(code, dto)
	}
}

// bindBody reads the request body in whichever format the client sent.
func bindBody(c *gin.Context, req any, msg proto.Message, fill func()) error {
	if !isProto(c.ContentType()) {
		if err := c.ShouldBindJSON(req); err != nil {
			return invalid("body", err.Error())
		}
		return nil
	}
	data, err := c.GetRawData()
	if err != nil {
		return invalid("body", err.Error())
	}
	if err := proto.Unmarshal(data, msg); err != nil {
		return invalid("body", err.Error())
	}
	fill()
	return nil
}
