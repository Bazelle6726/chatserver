# Performance

All numbers are from one laptop (Intel Core i5-1335U), with the load tester on the same machine as the server.

## JSON vs protobuf

Encoding a 100-message history:

```bash
go test ./internal/api -run XXX -bench Encode -benchmem
```

| | JSON | Protobuf |
|---|---|---|
| Size | 15,185 bytes | 7,884 bytes (52%) |
| Time to encode | ~46 µs | ~17 µs (~2.7× faster) |
| Memory per encode | 16 KB, 3 allocations | 8 KB, 1 allocation |

`-run XXX` skips the normal tests, since no test has that name.

## Load testing

With the server running:

```bash
go run ./cmd/loadtest                                      # 100 watchers, 10 posters × 20/s, 10s
go run ./cmd/loadtest -watchers 500 -posters 20 -rate 50   # heavier
go run ./cmd/loadtest -h                                   # all flags
```

It reports throughput, post latency percentiles, and how many messages reached every watcher:

| Watchers | Posts/s | p50 | p99 | Delivered |
|---|---|---|---|---|
| 100 | 199 | 1.76 ms | 6.54 ms | 100% |
| 500 | 998 | 9.92 ms | 15.65 ms | 100% |
| 1,000 | 2,245 | 1.14 ms | 75.99 ms | 6% |

At 1,000 watchers the server needed over 2 million sends a second. The hub dropped the slow watchers as designed, which is also why p50 fell.

## Profiling

`pprof` runs on `localhost:6060`. Take a CPU profile while a load test is running:

```bash
go tool pprof -top "http://localhost:6060/debug/pprof/profile?seconds=10"
go tool pprof -http=localhost:8081 "http://localhost:6060/debug/pprof/profile?seconds=10"   # flame graph
```

Under load, most of the CPU goes to network writes (~30%) and the garbage collector (~28%). The chat logic itself is only a few percent.

## The ring buffer

```bash
go test ./internal/chat -run XXX -bench History -benchmem
```

Profiling this benchmark showed that the room history spent ~97% of its time copying memory once it was full, because every new message shifted all the others down. Replacing it with a **ring buffer**, which overwrites the oldest message in place, made adding a message 48× faster at 1,000 messages (2,135 ns → 44 ns), and the cost is now the same for any history size.

For the server as a whole the gain is small, because the live profile showed the history was a small part of the total. Speeding up something that takes 2% of the time can only ever save 2%.
