# chatserver — Stage 1: domain types and in-memory store

Your job in this stage: make every test pass by filling in the `TODO(n)` function bodies.
Every TODO names the Go feature it practises (see the table below).

## Run it

```bash
go test ./...                       # everything (all fail at first, that's expected)
go test ./internal/chat -run Role   # just the tests whose names match "Role"
go test -v ./internal/chat          # verbose: see every test name
go run ./cmd/server                 # small demo once the store works
```

Needs Go 1.24 or newer (`go version`). Stage 3 uses `wg.Go`, which needs 1.25.

## Layout

```
cmd/server/main.go        demo program (later: starts Gin + gRPC)
internal/chat/            domain: IDs, enums, errors, User/Admin/Message, Room
internal/store/           Store interface + MemoryStore
```

`internal/` means no other module can import these packages — Go enforces that.

## Suggested order

Work package by package. `chat` first, because `store` depends on it.

| TODO | File | Practises |
|---|---|---|
| 1–3 | chat/enums.go | iota, switch, methods on non-struct types |
| 7–8 | chat/validate.go | for-range over runes, custom error types |
| 4–6 | chat/models.go | embedded pointer, pointer vs value receivers |
| 9–11 | chat/room.go | make, nil maps, comma-ok, delete |
| 12–13 | chat/room.go | append, copy, capacity, slice memory |
| 14 | chat/room.go | maps.Clone |
| 15 | store/idgen.go | closures |
| 16 | store/memory.go | variadic functional options |
| 17–20 | store/memory.go | fmt.Errorf with %w, errors.Is / errors.As |
| 21 | store/memory.go | random map order, slices.SortFunc |
| 22 | store/memory.go | putting it together |

Tips:
- Read the test for a function before writing it — the test is the spec.
- A `*ValidationError` is what `errors.As` looks for, so return `&ValidationError{...}`.
- TODO 12 has a capacity test. If it fails, look up how `append` grows a slice past its capacity, and why a sub-slice keeps the whole backing array alive.

## Done when

```
ok   chatserver/internal/chat
ok   chatserver/internal/store
```

and `go test -race ./...` is also clean. Then send me your code for review before stage 2 (Gin REST API).
