# Development

- [Requirements](#requirements)
- [Testing](#testing)
- [Regenerating the protobuf code](#regenerating-the-protobuf-code)
- [The web app](#the-web-app)

## Requirements

- **Go**, at the version in `go.mod` or newer.
- Optional:
  - **gcc**, to run tests with `-race` on Windows. The race detector needs cgo; [WinLibs](https://winlibs.com/) works, e.g. `winget install BrechtSanders.WinLibs.POSIX.UCRT`.
  - **protoc** with `protoc-gen-go` and `protoc-gen-go-grpc`, only if you change `chat.proto`.
  - **Node.js**, only if you change the React app.

## Testing

```bash
go test ./...                             # all tests
go test -race ./...                       # with the race detector: what every change should pass
go test -v ./internal/api -run Stream     # one area, verbose
```

| Area | How |
|---|---|
| Domain rules, validation, history | table-driven unit tests |
| Store, moderation list | many goroutines at once, with `-race` |
| HTTP API, admin API | `net/http/httptest`, no network or port |
| gRPC API | `grpc/test/bufconn`, an in-memory connection with a real server and client |
| Hub, worker pool, streams | every wait has a timeout, so a deadlock fails instead of hanging |
| Serving the web app | `testing/fstest.MapFS`, an in-memory file system |
| Load tester | runs against an in-memory server |

> **Windows:** after installing gcc, run `$env:CGO_ENABLED=1` in PowerShell before `go test -race`.

## Regenerating the protobuf code

The generated files (`chat.pb.go`, `chat_grpc.pb.go`) are committed, so you only need this after changing `chat.proto`:

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       internal/pb/chat.proto
```

## The web app

The React source is in `web/src/`, and the built app is in `web/dist/`. `web/embed.go` embeds the build into the server with `//go:embed dist`, and the API serves it for any path that no API route matches.

`dist/` is committed, so the server builds without Node.js. To change the app, install Node.js (`winget install OpenJS.NodeJS.LTS` on Windows), and with the Go server running in another terminal:

```bash
cd web
npm install        # once: downloads React and Vite
npm run dev        # development server on http://localhost:5173
```

Open **http://localhost:5173**. Vite passes API calls through to `:8080`, and every save shows up in the browser at once. When you're done:

```bash
npm run build      # rebuilds web/dist/
```

Restart the Go server to embed the new build, and commit `web/dist/` with your changes.
