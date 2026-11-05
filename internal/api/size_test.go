package api

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"chatserver/internal/chat"
	"chatserver/internal/pb"
	"chatserver/internal/pbconv"
)

// The same 100-message history, in both formats.
func sampleHistory() []chat.Message {
	at := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	ms := make([]chat.Message, 100)
	for i := range ms {
		ms[i] = chat.Message{
			Meta:   chat.Meta{ID: chat.MessageID(fmt.Sprintf("m-%d", i+1)), CreatedAt: at.Add(time.Duration(i) * time.Second)},
			Room:   "r-1",
			Author: "u-1",
			Kind:   chat.KindText,
			Text:   fmt.Sprintf("message number %d, about the length of a chat line", i+1),
		}
	}
	return ms
}

// Run with -v to see the sizes:
//
//	go test ./internal/api -run Smaller -v
func TestProtobufIsSmaller(t *testing.T) {
	ms := sampleHistory()
	j, err := json.Marshal(toMessageDTOs(ms))
	if err != nil {
		t.Fatal(err)
	}
	p, err := proto.Marshal(&pb.HistoryResponse{Messages: pbconv.Messages(ms)})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("100 messages: JSON %d bytes, protobuf %d bytes (%.0f%% of JSON)", len(j), len(p), 100*float64(len(p))/float64(len(j)))
	if len(p) >= len(j) {
		t.Errorf("protobuf (%d bytes) should be smaller than JSON (%d bytes)", len(p), len(j))
	}
}

// Benchmarks: run with
//
//	go test ./internal/api -run XXX -bench Encode -benchmem
//
// -run XXX skips the ordinary tests. b.Loop() runs the body as many times as
// it takes to get a stable time per operation.

func BenchmarkEncodeJSON(b *testing.B) {
	dtos := toMessageDTOs(sampleHistory())
	for b.Loop() {
		if _, err := json.Marshal(dtos); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeProto(b *testing.B) {
	msg := &pb.HistoryResponse{Messages: pbconv.Messages(sampleHistory())}
	for b.Loop() {
		if _, err := proto.Marshal(msg); err != nil {
			b.Fatal(err)
		}
	}
}
