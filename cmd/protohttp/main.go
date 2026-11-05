// Command protohttp talks to the HTTP API in protobuf instead of JSON, then
// fetches the same history in both formats and compares the sizes.
//
// Start the server first (go run ./cmd/server), then:
//
//	go run ./cmd/protohttp
//	go run ./cmd/protohttp -n 500
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"google.golang.org/protobuf/proto"

	"chatserver/internal/pb"
)

const mimeProto = "application/x-protobuf"

var base = flag.String("addr", "http://localhost:8080", "the HTTP API")

func main() {
	n := flag.Int("n", 100, "how many messages to post")
	flag.Parse()

	// A fresh name each run, so you can run it again without a 409.
	name := fmt.Sprintf("bot%d", time.Now().Unix()%100000)

	var user pb.User
	must(call("POST", "/users", &pb.CreateUserRequest{Name: name}, "", &user))
	fmt.Printf("created user %s (%s) in protobuf\n", user.GetId(), user.GetName())

	var room pb.Room
	must(call("POST", "/rooms", &pb.CreateRoomRequest{Name: name + "-room", MaxHistory: int32(*n)}, user.GetId(), &room))
	fmt.Printf("created room %s (%s)\n", room.GetId(), room.GetName())

	path := "/rooms/" + room.GetId()
	must(call("POST", path+"/members", nil, user.GetId(), nil))

	// The server allows 20 requests per second per user (the rate limiter
	// from stage 2), so pace the posts at 15 a second.
	fmt.Printf("posting %d messages (about %ds)...\n", *n, *n/15+1)
	tick := time.NewTicker(time.Second / 15)
	defer tick.Stop()
	for i := range *n {
		<-tick.C
		text := fmt.Sprintf("message %d, sent as protobuf over plain HTTP", i+1)
		must(call("POST", path+"/messages", &pb.PostMessageRequest{Text: text}, user.GetId(), nil))
	}
	fmt.Println()

	// The same history, asked for in each format.
	limit := fmt.Sprintf("?limit=%d", *n)
	jsonBody := fetch(path+"/messages"+limit, "application/json", user.GetId())
	protoBody := fetch(path+"/messages"+limit, mimeProto, user.GetId())

	var asJSON []map[string]any
	must(json.Unmarshal(jsonBody, &asJSON))
	var asProto pb.HistoryResponse
	must(proto.Unmarshal(protoBody, &asProto))

	fmt.Printf("history of %d messages:\n", len(asProto.GetMessages()))
	fmt.Printf("  JSON      %7d bytes\n", len(jsonBody))
	fmt.Printf("  protobuf  %7d bytes  (%.0f%% of JSON)\n", len(protoBody), 100*float64(len(protoBody))/float64(len(jsonBody)))
}

// call sends body as protobuf (nil = no body) and, if out isn't nil, decodes
// a protobuf reply into it.
func call(method, path string, body proto.Message, user string, out proto.Message) error {
	var data []byte
	if body != nil {
		var err error
		if data, err = proto.Marshal(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequest(method, *base+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", mimeProto)
	}
	req.Header.Set("Accept", mimeProto)
	if user != "" {
		req.Header.Set("X-User-ID", user)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	reply, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		// Errors come back as JSON, whatever we asked for.
		return fmt.Errorf("%s %s: %s %s", method, path, resp.Status, reply)
	}
	if out == nil {
		return nil
	}
	return proto.Unmarshal(reply, out)
}

func fetch(path, accept, user string) []byte {
	req, err := http.NewRequest("GET", *base+path, nil)
	must(err)
	req.Header.Set("Accept", accept)
	req.Header.Set("X-User-ID", user)
	resp, err := http.DefaultClient.Do(req)
	must(err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	must(err)
	return body
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
