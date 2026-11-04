// Command client is a small gRPC client for the chat server, for trying out
// all four kinds of call from the terminal. Run it with no arguments for help.
//
//	go run ./cmd/client create-user alice
//	go run ./cmd/client -user u-1 watch r-1
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"chatserver/internal/pb"
)

const usage = `usage: go run ./cmd/client [-addr localhost:9090] [-user u-1] <command> [args]

unary:
  create-user <name>          (no -user needed)
  create-room <name>
  rooms
  join <room>
  post <room> <text>
  history <room>
server streaming:
  watch <room>                prints messages as they arrive; Ctrl+C to stop
client streaming:
  import <room> <file>        posts every line of the file in one call
bidirectional streaming:
  chat <room>                 type lines to post them; see everyone's messages
`

func main() {
	addr := flag.String("addr", "localhost:9090", "server address")
	user := flag.String("user", "", "your user ID, e.g. u-1")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	// NewClient doesn't connect yet; the first call does. insecure = no TLS,
	// fine on localhost.
	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fail(err)
	}
	defer conn.Close()
	c := pb.NewChatServiceClient(conn)

	// The user ID travels as metadata, like the X-User-ID header over HTTP.
	ctx := context.Background()
	if *user != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-user-id", *user)
	}

	if err := run(ctx, c, args); err != nil {
		fail(err)
	}
}

func run(ctx context.Context, c pb.ChatServiceClient, args []string) error {
	need := func(n int) {
		if len(args) < n+1 {
			flag.Usage()
			os.Exit(2)
		}
	}
	// Unary calls get a timeout; streams last as long as you want.
	short, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	switch args[0] {
	case "create-user":
		need(1)
		u, err := c.CreateUser(short, &pb.CreateUserRequest{Name: args[1]})
		if err != nil {
			return err
		}
		fmt.Printf("created %s (%s), role %v\n", u.GetId(), u.GetName(), u.GetRole())
	case "create-room":
		need(1)
		r, err := c.CreateRoom(short, &pb.CreateRoomRequest{Name: args[1]})
		if err != nil {
			return err
		}
		fmt.Printf("created %s (%s)\n", r.GetId(), r.GetName())
	case "rooms":
		list, err := c.ListRooms(short, &pb.ListRoomsRequest{})
		if err != nil {
			return err
		}
		for _, r := range list.GetRooms() {
			fmt.Printf("%-6s %-20s %d members\n", r.GetId(), r.GetName(), r.GetMembers())
		}
	case "join":
		need(1)
		if _, err := c.JoinRoom(short, &pb.JoinRoomRequest{RoomId: args[1]}); err != nil {
			return err
		}
		fmt.Println("joined", args[1])
	case "post":
		need(2)
		m, err := c.PostMessage(short, &pb.PostMessageRequest{RoomId: args[1], Text: args[2]})
		if err != nil {
			return err
		}
		printMessage(m)
	case "history":
		need(1)
		h, err := c.History(short, &pb.HistoryRequest{RoomId: args[1]})
		if err != nil {
			return err
		}
		for _, m := range h.GetMessages() {
			printMessage(m)
		}
	case "watch":
		need(1)
		return watch(ctx, c, args[1])
	case "import":
		need(2)
		return importFile(ctx, c, args[1], args[2])
	case "chat":
		need(1)
		return converse(ctx, c, args[1])
	default:
		flag.Usage()
		os.Exit(2)
	}
	return nil
}

func watch(ctx context.Context, c pb.ChatServiceClient, room string) error {
	stream, err := c.Watch(ctx, &pb.WatchRequest{RoomId: room})
	if err != nil {
		return err
	}
	fmt.Println("watching", room, "- Ctrl+C to stop")
	for {
		m, err := stream.Recv()
		if err != nil {
			return err
		}
		printMessage(m)
	}
}

func importFile(ctx context.Context, c pb.ChatServiceClient, room, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	stream, err := c.Import(ctx)
	if err != nil {
		return err
	}
	lines := bufio.NewScanner(f)
	for lines.Scan() {
		if err := stream.Send(&pb.PostMessageRequest{RoomId: room, Text: lines.Text()}); err != nil {
			break // the server ended the call early; CloseAndRecv says why
		}
	}
	sum, err := stream.CloseAndRecv()
	if err != nil {
		return err
	}
	fmt.Printf("imported %d messages into %s\n", sum.GetPosted(), room)
	return nil
}

func converse(ctx context.Context, c pb.ChatServiceClient, room string) error {
	stream, err := c.Converse(ctx)
	if err != nil {
		return err
	}
	if err := stream.Send(&pb.ConverseRequest{Action: &pb.ConverseRequest_JoinRoomId{JoinRoomId: room}}); err != nil {
		return err
	}
	fmt.Println("chatting in", room, "- type a line and press Enter; Ctrl+Z then Enter to leave")

	// Like the server: one goroutine sends (reading your typing), this one receives.
	go func() {
		in := bufio.NewScanner(os.Stdin)
		for in.Scan() {
			if err := stream.Send(&pb.ConverseRequest{Action: &pb.ConverseRequest_Text{Text: in.Text()}}); err != nil {
				return
			}
		}
		stream.CloseSend() // end of input: tell the server we're done
	}()
	for {
		m, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		printMessage(m)
	}
}

func printMessage(m *pb.Message) {
	at := time.UnixMilli(m.GetCreatedAtUnixMilli()).Format("15:04:05")
	fmt.Printf("[%s] %s %s: %s\n", at, m.GetRoomId(), m.GetAuthorId(), m.GetText())
}

// fail prints a gRPC error as its code and message, e.g.
// "NotFound: room "r-9": room not found".
func fail(err error) {
	if s, ok := status.FromError(err); ok {
		fmt.Fprintf(os.Stderr, "%v: %s\n", s.Code(), s.Message())
	} else {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(1)
}
