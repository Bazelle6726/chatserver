# API reference

- [A quick tour with curl](#a-quick-tour-with-curl)
- [HTTP API](#http-api)
- [Live feed (SSE)](#live-feed-sse)
- [Moderation and the admin API](#moderation-and-the-admin-api)
- [gRPC API](#grpc-api)
- [Protobuf over HTTP](#protobuf-over-http)

On Windows PowerShell, type `curl.exe` rather than `curl`, so you get real curl and not PowerShell's alias.

## A quick tour with curl

**1. Create a user.** No login is needed for this:

```bash
curl -X POST localhost:8080/users -H "Content-Type: application/json" -d '{"name":"alice"}'
# {"id":"u-1","name":"alice","role":"member"}
```

**2. Create a room and join it.** From here on, send your user ID in the `X-User-ID` header:

```bash
curl -X POST localhost:8080/rooms -H "X-User-ID: u-1" -H "Content-Type: application/json" -d '{"name":"general"}'
curl -X POST localhost:8080/rooms/r-1/members -H "X-User-ID: u-1"
```

**3. Watch the room live.** Run this in a second terminal and leave it running:

```bash
curl -N -H "X-User-ID: u-1" localhost:8080/rooms/r-1/stream
```

**4. Post.** Back in the first terminal:

```bash
curl -X POST localhost:8080/rooms/r-1/messages -H "X-User-ID: u-1" -H "Content-Type: application/json" -d '{"text":"hello live"}'
curl -X POST localhost:8080/rooms/r-1/messages -H "X-User-ID: u-1" -H "Content-Type: application/json" -d '{"text":"cheap scam here"}'
```

Both appear in the watcher immediately. The second arrives as `[removed]`, and the history shows it the same way:

```bash
curl -H "X-User-ID: u-1" "localhost:8080/rooms/r-1/messages?limit=10"
```

**5. The same over gRPC:**

```bash
go run ./cmd/client -user u-1 watch r-1               # terminal 2
go run ./cmd/client -user u-1 post r-1 "hello gRPC"   # terminal 1
```

HTTP and gRPC share the same hub, so a gRPC watcher sees HTTP posts and the other way round.

## HTTP API

Base URL: `http://localhost:8080`. Bodies are JSON unless you ask for [protobuf](#protobuf-over-http).

### Authentication

All routes except the two user routes need an existing user's ID, in the `X-User-ID` header or, for clients that can't set headers, the `user` query parameter (`?user=u-1`). Without one you get `401`. The admin routes use a token instead (see [below](#moderation-and-the-admin-api)).

> This trusts the client, which is fine for a learning project. A real service would use signed tokens, and keep them out of URLs, which end up in logs and browser history.

### Endpoints

| Method | Path | Auth | Body | Success |
|---|---|---|---|---|
| `POST` | `/users` | – | `{"name": "alice"}` | `201` user |
| `GET` | `/users/:id` | – | – | `200` user |
| `POST` | `/rooms` | user | `{"name": "general", "max_history": 100}` | `201` room |
| `GET` | `/rooms` | user | – | `200` list of rooms, sorted by name |
| `POST` | `/rooms/:id/members` | user | – | `204` (you join the room) |
| `POST` | `/rooms/:id/messages` | user | `{"text": "hello"}` | `201` message |
| `GET` | `/rooms/:id/messages?limit=50` | member | – | `200` list of messages, oldest first |
| `GET` | `/rooms/:id/stream` | member | – | `200` event stream |
| `GET` | `/admin/banned-words` | admin | – | `200` `{"words": [...]}` |
| `POST` | `/admin/banned-words` | admin | `{"word": "spam"}` | `201` `{"word": "spam"}` |
| `DELETE` | `/admin/banned-words/:word` | admin | – | `204` |
| `GET` | any other path | – | – | the web app's files |

**Rules:**

- **Names** (users and rooms): 3–20 characters, using letters, digits, `-` and `_`, and unique.
  - A name can't mix Latin, Cyrillic and Greek letters, so `pаypal` with a Cyrillic `а` is refused.
  - Uniqueness ignores case and look-alike letters: once `alice` exists, `Alice`, `ＡＬＩＣＥ` (fullwidth) and an all-Cyrillic look-alike are taken too.
- **Membership**: you must join a room (`POST /rooms/:id/members`) before you can post to it, read its history or watch its stream.
- **Messages**: not blank, at most 500 characters.
- **Body size**: at most 64 KiB, then `413`.
- **`max_history`**: 1 to 1000; omitted or 0 means 100.
- **`limit`**: 1 to 500, default 50.
- **Rate limit**: 20 requests per second per user on user routes, then `429`.

### Response shapes

```json
// user
{"id": "u-1", "name": "alice", "role": "member"}

// room
{"id": "r-1", "name": "general", "members": 3}

// message
{"id": "m-7", "room": "r-1", "author": "u-1", "kind": "text",
 "text": "hello", "created_at": "2026-11-09T14:02:11.123Z"}
```

### Errors

Errors are always JSON. Validation errors also name the field:

```json
{"error": "invalid name: name must be between 3 and 20 characters long", "field": "name"}
```

| Status | When |
|---|---|
| `400` | invalid body, name, text, limit or word |
| `401` | missing or unknown user ID, or a missing or wrong admin token |
| `403` | posting to, reading or watching a room you haven't joined |
| `404` | unknown user, room or path, or the admin API is off |
| `409` | name already taken (or one that looks the same), or already a member |
| `413` | request body over 64 KiB |
| `429` | rate limit exceeded, or too many wrong admin tokens |
| `500` | anything unexpected; the body is just `{"error": "internal error"}` and the details go to the server log |

## Live feed (SSE)

`GET /rooms/:id/stream` uses [Server-Sent Events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events): a normal HTTP response that stays open, with one event per message.

```
: connected

event: message
data: {"id":"m-1","room":"r-1","author":"u-1","kind":"text","text":"hello live","created_at":"..."}
```

In a browser, which is how the web app does it:

```js
// EventSource can't set custom headers, so the user ID goes in the URL.
const es = new EventSource("/rooms/r-1/stream?user=u-1");
es.addEventListener("message", e => console.log(JSON.parse(e.data)));
```

Each watcher has a fixed-size buffer. A watcher that falls too far behind has its stream closed rather than slowing the room down; it can reconnect and catch up from the history.

## Moderation and the admin API

Messages are checked against [`banned_words.txt`](../banned_words.txt), loaded at startup from the folder the server runs in.

- **Format:** one word per line. Blank lines and lines starting with `#` are ignored.
- **Matching:** whole words, ignoring case. With `scam` banned, "SCAM!" is caught but "scampi" isn't, so list every form you want caught.
- **Disguises are caught too.** Before matching, text is folded ([`internal/textnorm`](../internal/textnorm/textnorm.go) and [`internal/moderation`](../internal/moderation/moderation.go)):
  - look-alike letters from other scripts become Latin: `ѕсаm` (Cyrillic) → `scam`
  - fullwidth and other compatibility forms become plain: `ＳＣＡＭ` → `scam`
  - invisible characters are dropped: `sc` + zero-width space + `am` → `scam`
  - leetspeak is undone: `sc4m`, `$cam`, `sc@m` → `scam`
  - spaced-out letters are glued back together: `s c a m` and `s.c.a.m` → `scam`
- **Effect:** a matching message is shown as `[removed]` in the live feed and in the history, over both HTTP and gRPC. The store keeps the original text.
- **When:** history is checked as it's read, so banning a word also hides older messages that used it, and unbanning brings them back.

A word list still won't catch everything: a word split differently (`sc am`) or a look-alike missing from the table gets through. The full list of look-alikes is Unicode's [confusables.txt](https://www.unicode.org/reports/tr39/). For a wider word list, start from an open-source one such as LDNOOBW.

### Admin API

The admin API is **off unless `CHAT_ADMIN_TOKEN` is set** to at least 16 characters when the server starts; while it's off, its routes answer `404`. Each request sends the token in the `X-Admin-Token` header.

After **5 wrong tokens in a minute** from one IP address, that address gets `429` until the minute is up, even with the right token. The server uses the connection's real address and ignores `X-Forwarded-For`, so a client can't dodge the lockout by claiming a new IP. Behind a reverse proxy, add the proxy's address to `SetTrustedProxies` in [`router.go`](../internal/api/router.go).

```bash
TOKEN=pick-a-long-secret

curl -H "X-Admin-Token: $TOKEN" localhost:8080/admin/banned-words
curl -X POST localhost:8080/admin/banned-words -H "X-Admin-Token: $TOKEN" \
     -H "Content-Type: application/json" -d '{"word":"fraud"}'
curl -X DELETE -H "X-Admin-Token: $TOKEN" localhost:8080/admin/banned-words/fraud
```

In PowerShell:

```powershell
$h = @{ "X-Admin-Token" = $env:CHAT_ADMIN_TOKEN }
Invoke-RestMethod localhost:8080/admin/banned-words -Headers $h
Invoke-RestMethod localhost:8080/admin/banned-words -Method Post -Headers $h `
    -ContentType "application/json" -Body '{"word":"fraud"}'
Invoke-RestMethod localhost:8080/admin/banned-words/fraud -Method Delete -Headers $h
```

- **Words** are trimmed, then must be 1–50 letters or digits. Anything else could never match, so it gets `400`.
- **Words are stored folded**, the same way messages are: adding `Sc4m` stores and lists `scam`, and the response's `word` says what was stored.
- **Changes take effect at once**, for new messages and in the history: banning a word also hides older messages that used it, and unbanning shows them again. Changes are saved to `banned_words.txt`. Saving rewrites the file as a plain sorted list, so its comments are lost after the first change. Each save writes a temporary file and then swaps it in, so two admins saving at once, or a crash mid-save, can't leave a broken file.
- **Removing** a word that isn't on the list still answers `204`. Any spelling of a word removes it: `DELETE /admin/banned-words/SC4M` removes `scam`.

The web app's **Moderation (admins)** panel uses these same routes.

## gRPC API

Defined in [`internal/pb/chat.proto`](../internal/pb/chat.proto) and served on `localhost:9090`. The user ID travels as `x-user-id` metadata; `CreateUser` and `GetUser` don't need it.

| Call | Kind | Does |
|---|---|---|
| `CreateUser`, `GetUser` | unary | users |
| `CreateRoom`, `ListRooms`, `JoinRoom` | unary | rooms |
| `PostMessage`, `History` | unary | messages |
| `Watch` | server streaming | receive a room's messages live |
| `Import` | client streaming | send many messages, get one summary |
| `Converse` | bidirectional | post and receive on one call; the first message names the room |

Errors come back as gRPC status codes that match the HTTP ones: `InvalidArgument`, `Unauthenticated`, `PermissionDenied`, `NotFound`, `AlreadyExists`, `Internal`. As over HTTP, `History` and `Watch` need you to have joined the room (else `PermissionDenied`), and an `Internal` error's message is just "internal error". `Converse` joins you to the room named in its first message, if you aren't a member yet.

### The terminal client

```bash
go run ./cmd/client                                   # help
go run ./cmd/client create-user bob
go run ./cmd/client -user u-2 create-room random
go run ./cmd/client -user u-2 rooms
go run ./cmd/client -user u-2 join r-2
go run ./cmd/client -user u-2 post r-2 "hi"
go run ./cmd/client -user u-2 history r-2
go run ./cmd/client -user u-2 watch r-2               # server streaming; Ctrl+C to stop
go run ./cmd/client -user u-2 import r-2 lines.txt    # client streaming; one message per line
go run ./cmd/client -user u-2 chat r-2                # bidirectional; type to post
```

The server registers gRPC reflection, so [grpcurl](https://github.com/fullstorydev/grpcurl) works without the `.proto` file: `grpcurl -plaintext localhost:9090 list`.

## Protobuf over HTTP

The HTTP routes also speak protobuf, using the message types from `chat.proto`. Two headers decide the format, independently:

| Header | Meaning | Values |
|---|---|---|
| `Content-Type` | format of the request body | `application/json` (default), `application/x-protobuf` |
| `Accept` | format you want back | `application/json` (default), `application/x-protobuf` |

Error responses are always JSON. `go run ./cmd/protohttp` posts in protobuf, then fetches the same history in both formats and compares the sizes. The measurements are in [PERFORMANCE.md](PERFORMANCE.md#json-vs-protobuf).
