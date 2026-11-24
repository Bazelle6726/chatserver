// ChatClient is the component a game talks to: give it a session token, and
// it connects, joins rooms, and raises events on the main thread for every
// message. It reconnects by itself if the connection drops, and joins the
// same rooms again.
//
//     chat.Connect(token);                 // the token your game backend got from /game/sessions
//     chat.JoinByName("match-8841");
//     chat.MessageReceived += m => Debug.Log($"{m.author_name}: {m.text}");
//     chat.Send(roomId, "gg");
using System;
using System.Collections.Generic;
using UnityEngine;

namespace ChatServer.Chat
{
    [DisallowMultipleComponent]
    public class ChatClient : MonoBehaviour
    {
        // Close codes the server uses for a failed login (see ws.go).
        const int CloseBadLogin = 4401;     // the token was refused: retrying won't help
        const int CloseTryAgainLater = 1013; // rate limited or a server problem: retry later

        // A connection that lasts this long is "healthy": the reconnect delay
        // starts again from 1 second. Shorter ones keep backing off, so a
        // connection that fails right after logging in can't loop quickly.
        const float HealthySeconds = 10f;

        [Tooltip("The chat server's WebSocket address. Use wss:// once the server is behind HTTPS.")]
        public string serverUrl = "ws://localhost:8080/ws";

        [Tooltip("Rooms to join by name as soon as the client connects, e.g. a lobby.")]
        public string[] joinOnConnect = new string[0];

        [Tooltip("How many recent messages to fetch when joining a room (1-500).")]
        [Range(1, 500)]
        public int historyLimit = 50;

        [Tooltip("Longest wait between reconnect attempts, in seconds.")]
        public float maxReconnectDelay = 30f;

        /// <summary>Logged in; carries who you are.</summary>
        public event Action<ChatUser> Connected;
        /// <summary>The connection dropped. willRetry is false after Disconnect() or a failed login.</summary>
        public event Action<string, bool> Disconnected;
        /// <summary>The token was refused (unknown or expired). Get a new one, then call Connect again.</summary>
        public event Action<string> AuthFailed;
        /// <summary>A room was joined, with its recent history (oldest first).</summary>
        public event Action<ChatRoom, ChatMessage[]> Joined;
        /// <summary>A new message in a joined room. Your own arrive here too.</summary>
        public event Action<ChatMessage> MessageReceived;
        /// <summary>You stopped receiving a room (you left, or were dropped for reading too slowly).</summary>
        public event Action<string, string> Left;
        /// <summary>The server refused a request (rate limited, not allowed, ...).</summary>
        public event Action<ChatError> Error;

        /// <summary>Who you're logged in as, once connected.</summary>
        public ChatUser Me { get; private set; }

        /// <summary>True while connected and logged in.</summary>
        public bool IsConnected => _connection != null && _connection.IsOpen && Me != null;

        /// <summary>Rooms joined on this connection, by ID.</summary>
        public IReadOnlyDictionary<string, ChatRoom> Rooms => _rooms;

        readonly Dictionary<string, ChatRoom> _rooms = new Dictionary<string, ChatRoom>();
        readonly List<ChatRequest> _pending = new List<ChatRequest>();                    // to send once logged in
        readonly Dictionary<string, ChatRequest> _inflight = new Dictionary<string, ChatRequest>(); // joins sent, not yet answered, by ref
        ChatConnection _connection;
        string _token;
        bool _wantConnected;
        bool _joinedOnConnect; // joinOnConnect runs once per Connect(), not on every reconnect
        float _reconnectAt = -1;
        float _reconnectDelay = 1;
        float _connectedAt;
        int _nextRef;

        /// <summary>Connects with a session token (from your game backend, or ChatDevAuth when testing).</summary>
        public void Connect(string token)
        {
            if (string.IsNullOrEmpty(token))
                throw new ArgumentException("a session token is needed", nameof(token));
            _token = token;
            _wantConnected = true;
            _joinedOnConnect = false;
            _reconnectAt = -1; // a retry scheduled for the old token must not replace this connection
            _reconnectDelay = 1;
            Open();
        }

        /// <summary>
        /// Disconnects, stops reconnecting, and forgets the rooms, so a later
        /// Connect (perhaps as another player) starts clean.
        /// </summary>
        public void Disconnect()
        {
            _wantConnected = false;
            _reconnectAt = -1;
            _rooms.Clear();
            _pending.Clear();
            _inflight.Clear();
            Close("disconnected", false);
        }

        /// <summary>
        /// Joins a room by name: a lobby (created if missing) or a match room
        /// your game backend added you to. Before connecting, it's queued.
        /// </summary>
        public string JoinByName(string roomName) =>
            Request(new ChatRequest { type = "join", room_name = roomName, limit = HistoryLimit });

        /// <summary>Joins a room by ID.</summary>
        public string JoinById(string roomId) =>
            Request(new ChatRequest { type = "join", room_id = roomId, limit = HistoryLimit });

        /// <summary>Stops receiving a room (you stay a member).</summary>
        public void Leave(string roomId)
        {
            _rooms.Remove(roomId);
            if (IsConnected) // offline, forgetting it is enough: it won't be rejoined
                Request(new ChatRequest { type = "leave", room_id = roomId });
        }

        /// <summary>
        /// Sends a message to a joined room. Returns the request's ref; a
        /// refusal arrives as an Error with the same ref.
        /// </summary>
        public string Send(string roomId, string text)
        {
            if (!IsConnected)
            {
                var r = NextRef();
                Raise(() => Error?.Invoke(new ChatError("not_connected", "not connected to the chat server", r)));
                return r;
            }
            return Request(new ChatRequest { type = "send", room_id = roomId, text = text });
        }

        // The server accepts 1-500; the Inspector's range doesn't stop code setting more.
        int HistoryLimit => Mathf.Clamp(historyLimit, 1, 500);

        string NextRef() => (++_nextRef).ToString();

        string Request(ChatRequest request)
        {
            request.@ref = NextRef();
            if (IsConnected)
                Transmit(request);
            else
                _pending.Add(request);
            return request.@ref;
        }

        async void Transmit(ChatRequest request)
        {
            // Remember joins until answered, so one lost to a dropped
            // connection is sent again after reconnecting.
            if (request.type == "join")
                _inflight[request.@ref] = request;
            var connection = _connection;
            try
            {
                await connection.SendAsync(request);
            }
            catch (Exception e)
            {
                // The connection is going; the receive side reports it.
                Debug.LogWarning($"[chat] send failed: {e.Message}");
            }
        }

        async void Open()
        {
            Close(null, false);
            ChatConnection connection;
            try
            {
                connection = new ChatConnection(new Uri(serverUrl));
            }
            catch (Exception e)
            {
                Debug.LogError($"[chat] can't connect to {serverUrl}: {e.Message}");
                return;
            }
            _connection = connection;
            try
            {
                await connection.ConnectAsync(_token);
            }
            catch (Exception e)
            {
                if (_connection == connection)
                    Lost($"could not connect: {e.Message}");
            }
        }

        // Closes the connection. Joins still unanswered go back in the queue.
        void Close(string reason, bool willRetry)
        {
            if (_connection == null)
                return;
            _connection.Dispose();
            _connection = null;
            _pending.InsertRange(0, _inflight.Values);
            _inflight.Clear();
            var wasConnected = Me != null;
            Me = null;
            if (reason != null && wasConnected)
                Raise(() => Disconnected?.Invoke(reason, willRetry));
        }

        // The connection failed or dropped: try again later, if still wanted.
        void Lost(string reason)
        {
            var retry = _wantConnected;
            if (retry)
            {
                // Schedule first, so a throwing Disconnected handler can't stop it.
                _reconnectAt = Time.unscaledTime + _reconnectDelay;
                Debug.Log($"[chat] {reason}; retrying in {_reconnectDelay:0}s");
                _reconnectDelay = Mathf.Min(_reconnectDelay * 2, maxReconnectDelay);
            }
            Close(reason, retry);
        }

        void Update()
        {
            if (_reconnectAt >= 0 && Time.unscaledTime >= _reconnectAt)
            {
                _reconnectAt = -1;
                Open();
            }
            if (_reconnectDelay > 1 && IsConnected && Time.unscaledTime - _connectedAt > HealthySeconds)
                _reconnectDelay = 1;
            if (_connection == null)
                return;

            // Handle at most a frame's worth, so a burst can't stall the game.
            for (var i = 0; i < 200 && _connection != null && _connection.TryTake(out var incoming); i++)
            {
                if (incoming.Json == null)
                    Ended(incoming);
                else
                    Handle(incoming.Json);
            }
        }

        void Ended(ChatIncoming end)
        {
            var reason = end.CloseReason ?? "connection closed";
            if (end.CloseStatus.HasValue && (int)end.CloseStatus.Value == CloseBadLogin)
            {
                // A bad or expired token: retrying won't help.
                _wantConnected = false;
                _reconnectAt = -1;
                Close(reason, false);
                Raise(() => AuthFailed?.Invoke(reason));
                return;
            }
            // Everything else, including CloseTryAgainLater (rate limited at
            // login, or a server problem), is worth retrying with backoff.
            Lost(reason);
        }

        void Handle(string json)
        {
            ChatFrame f;
            try
            {
                f = JsonUtility.FromJson<ChatFrame>(json);
            }
            catch (Exception e)
            {
                Debug.LogWarning($"[chat] unreadable frame: {e.Message}");
                return;
            }
            switch (f.type)
            {
                case "ready":
                    Me = f.user;
                    _connectedAt = Time.unscaledTime;
                    Rejoin(); // before the event, so a throwing handler can't stop it
                    Raise(() => Connected?.Invoke(f.user));
                    break;
                case "joined":
                    Answered(f.@ref);
                    if (f.room != null && !string.IsNullOrEmpty(f.room.id))
                        _rooms[f.room.id] = f.room;
                    Raise(() => Joined?.Invoke(f.room, f.history ?? new ChatMessage[0]));
                    break;
                case "message":
                    Raise(() => MessageReceived?.Invoke(f.message));
                    break;
                case "left":
                    _rooms.Remove(f.room_id);
                    // Dropped for reading too slowly: join again to catch up.
                    if (f.reason != null && f.reason.StartsWith("dropped", StringComparison.Ordinal))
                        JoinById(f.room_id);
                    Raise(() => Left?.Invoke(f.room_id, f.reason));
                    break;
                case "error":
                    Answered(f.@ref);
                    Raise(() => Error?.Invoke(new ChatError(f.code, f.error, f.@ref)));
                    break;
                // "sent" and "pong" need nothing: your message arrives as "message" too.
            }
        }

        void Answered(string @ref)
        {
            if (!string.IsNullOrEmpty(@ref))
                _inflight.Remove(@ref);
        }

        // After logging in: rooms from before a reconnect, then anything asked
        // for while offline, then (the first time only) joinOnConnect.
        void Rejoin()
        {
            var requests = new List<ChatRequest>();
            var rejoining = new HashSet<string>();
            foreach (var id in _rooms.Keys)
            {
                requests.Add(new ChatRequest { type = "join", room_id = id, limit = HistoryLimit });
                rejoining.Add(id);
            }
            _rooms.Clear();
            foreach (var r in _pending)
                if (!(r.type == "join" && rejoining.Contains(r.room_id ?? "")))
                    requests.Add(r);
            _pending.Clear();
            if (!_joinedOnConnect)
            {
                _joinedOnConnect = true;
                foreach (var name in joinOnConnect)
                    if (!string.IsNullOrWhiteSpace(name))
                        requests.Add(new ChatRequest { type = "join", room_name = name, limit = HistoryLimit });
            }
            foreach (var r in requests)
            {
                if (string.IsNullOrEmpty(r.@ref))
                    r.@ref = NextRef();
                Transmit(r);
            }
        }

        // Game code subscribed to the events; an exception there is logged,
        // never allowed to break the client.
        static void Raise(Action raise)
        {
            try
            {
                raise();
            }
            catch (Exception e)
            {
                Debug.LogException(e);
            }
        }

        // Quitting or destroyed: close without raising events, so listeners
        // that are being torn down too aren't called.
        void Shutdown()
        {
            _wantConnected = false;
            _reconnectAt = -1;
            if (_connection != null)
            {
                _connection.Dispose();
                _connection = null;
            }
            Me = null;
        }

        void OnDestroy() => Shutdown();
        void OnApplicationQuit() => Shutdown();
    }
}
