# Chat Server Client for Unity

In-game chat for the Go chat server in this repository: a WebSocket client that reconnects by itself, and a ready-made chat box.

- **`ChatClient`**: the connection. Give it a session token; it logs in, joins rooms, and raises events on the main thread.
- **`ChatPanel`**: a chat box that builds its own UI (no prefab). Enter or T to type, Enter to send, Esc to close, Tab to switch rooms. Messages fade after 10 seconds.
- **`ChatDevAuth`**: logs in with a name and password, for trying it out before your game backend is wired up.

Requires Unity 2021.3 or newer, and the chat server running (`go run ./cmd/server` from the repository root).

## Install

In Unity: **Window > Package Manager > + > Add package from git URL**, and paste:

```
https://github.com/Bazelle6726/chatserver.git?path=/unity/com.chatserver.chat
```

Or, with the repository on disk: **+ > Add package from disk** and pick `unity/com.chatserver.chat/package.json`.

## Try it in 2 minutes

1. Start the server: `go run ./cmd/server`
2. Unity 2022.1 and newer block plain `http://` by default, which the Quickstart's test login uses. In **Project Settings > Player > Other Settings**, set **Allow downloads over HTTP** to **Allowed in development builds**. (The game's own traffic over `ws://` isn't affected; in production use `https://` and `wss://`.)
3. In Package Manager, open this package's **Samples** tab and import **Quickstart**.
4. Create an empty GameObject, add the **ChatQuickstart** component, press Play.
5. Press **Enter** or **T**, type, press **Enter**.

To chat with someone, build the game and run it next to the Editor, with a different **Chat Name** on the Quickstart component.

## Use it in a real game

Players shouldn't need a chat password. Your **game backend** signs them in with the game key, and hands each client its token:

```
Game client ──login──► your backend ──POST /game/sessions──► chat server
     ▲                      │              (X-Game-Key)          │
     └──────token───────────┘◄─────────{user, token}─────────────┘
Game client ──WebSocket /ws (token)──► chat server
```

**1. Server.** Start the chat server with a game key of 16+ characters (keep it secret, on your servers only):

```powershell
$env:CHAT_GAME_KEY = "a-long-random-secret-for-your-backend"
go run ./cmd/server -http-addr :8080
```

**2. Your backend**, when a player logs in to your game:

```
POST http://chat-server:8080/game/sessions
X-Game-Key: <CHAT_GAME_KEY>
{"player_id": "steam:76561198000000001", "name": "Ace"}

→ {"user": {"id": "u-3f9c...", "name": "Ace", "role": "member"}, "token": "Xq3..."}
```

Send `token` to that player's game client. The same `player_id` always maps to the same chat user.

**3. Your backend**, when a match starts, creates its private room and adds the players:

```
POST http://chat-server:8080/game/rooms
X-Game-Key: <CHAT_GAME_KEY>
{"name": "match-8841", "players": ["steam:76561198000000001", "steam:76561198000000002"]}
```

Only those players can see, read or post in `match-8841`. Call it again to add late joiners. A team chat is just another room (`match-8841-red`).

**4. The game client:**

```csharp
using ChatServer.Chat;

var chat = gameObject.AddComponent<ChatClient>();   // or add it in the Inspector
chat.serverUrl = "wss://chat.example.com/ws";
var panel = gameObject.AddComponent<ChatPanel>();

chat.Connect(tokenFromYourBackend);
chat.JoinByName("match-8841");                       // and "lobby", "match-8841-red", ...

// Stop moving the player while they type:
panel.TypingChanged += typing => playerInput.enabled = !typing;
```

## ChatClient

| Member | What it does |
|---|---|
| `serverUrl` | `ws://host:8080/ws`, or `wss://...` behind HTTPS |
| `joinOnConnect` | room names to join when it first connects |
| `historyLimit` | recent messages fetched on join (1-500) |
| `Connect(token)` / `Disconnect()` | start / stop. It reconnects by itself (1s, 2s, 4s ... up to 30s), rejoins its rooms, and resends joins that were in flight. `Disconnect()` forgets the rooms, so the next `Connect` starts clean |
| `JoinByName(name)` / `JoinById(id)` | join a lobby (created if missing) or a match room you were added to |
| `Send(roomId, text)` / `Leave(roomId)` | post (500 characters max; returns a ref that a refusal's `Error` carries) / stop receiving a room |
| `Connected`, `Disconnected`, `AuthFailed` | connection events. `AuthFailed` means the token was refused (unknown or expired) and the client has stopped: get a new token and call `Connect`. A temporary problem (a rate limit at login, a server error) just retries. An exception thrown in your handler is logged and never breaks the client |
| `Joined(room, history)`, `MessageReceived(message)`, `Left(roomId, reason)` | room events. Your own messages arrive in `MessageReceived` too |
| `Error(error)` | a refused request: `error.Code` is `rate_limited`, `forbidden`, `not_found`, `invalid`, `not_connected`, ... |
| `Me`, `Rooms`, `IsConnected` | state |

Messages carry `author_name`, so you can show names without looking anyone up. A message with a banned word arrives as `[removed]`.

## ChatPanel

Everything is in the Inspector: font, size, position, colours, fade time, keys. It opens only when none of your own text fields (uGUI or TextMeshPro) has focus, closes if focus moves elsewhere, and on mobile the keyboard's Done sends. Its canvas is a root object of its own (so a Canvas you put it under can't restyle it), and it follows the panel into `DontDestroyOnLoad`. It works with the old Input Manager, and with the new Input System (the package is detected automatically). If neither is enabled, call `Open()` and `Close()` from your own input code.

It's built from standard uGUI parts (`Canvas`, `ScrollRect`, `InputField`, `Text`), so it's easy to restyle or copy as the start of your own UI.

## Limits and notes

- **No WebGL yet.** The client uses .NET's `ClientWebSocket`, which works in the Editor and on Windows, macOS, Linux, Android and iOS, but not in WebGL builds. WebGL needs a JavaScript WebSocket bridge (for example the NativeWebSocket package) in place of `ChatConnection`; the server side already supports browsers.
- **Use `wss://` in production.** Put the server behind HTTPS (a reverse proxy) so tokens and messages are encrypted.
- **Rate limits** apply: by default 20 messages a second per player. Raise them with the server's `-posts-per-second` and `-requests-per-second` flags.
- **Restarting the server** currently wipes users, rooms, history and sessions (it keeps everything in memory). Clients reconnect, get `AuthFailed`, and need a new token from your backend.
