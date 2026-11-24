// One WebSocket connection to the chat server, without any Unity game-loop
// code. ChatClient wraps it for use in a scene.
//
// Threading: the socket is read on a background task. Nothing received is
// handed to game code from there; frames are queued, and the owner takes
// them on the main thread with TryTake (ChatClient does that in Update).
using System;
using System.Collections.Concurrent;
using System.IO;
using System.Net.WebSockets;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using UnityEngine;

namespace ChatServer.Chat
{
    /// <summary>Something that arrived on the connection: a frame, or the end of it.</summary>
    public struct ChatIncoming
    {
        /// <summary>A frame's JSON, or null when the connection ended.</summary>
        public string Json;
        /// <summary>How the connection ended (null if it failed without a close).</summary>
        public WebSocketCloseStatus? CloseStatus;
        /// <summary>The server's reason for closing, or the error.</summary>
        public string CloseReason;
    }

    public sealed class ChatConnection : IDisposable
    {
        const int MaxFrameBytes = 1 << 20; // a joined frame with 500 messages of history fits easily

        readonly Uri _uri;
        readonly ClientWebSocket _socket = new ClientWebSocket();
        readonly CancellationTokenSource _cts = new CancellationTokenSource();
        readonly ConcurrentQueue<ChatIncoming> _incoming = new ConcurrentQueue<ChatIncoming>();
        readonly SemaphoreSlim _sendLock = new SemaphoreSlim(1, 1); // a WebSocket allows one send at a time
        int _ended; // 1 once the end has been queued

        public ChatConnection(Uri uri)
        {
            _uri = uri;
            _socket.Options.KeepAliveInterval = TimeSpan.FromSeconds(20);
        }

        /// <summary>True while the socket is open.</summary>
        public bool IsOpen => _socket.State == WebSocketState.Open;

        /// <summary>
        /// Connects and logs in with token. The server answers with a "ready"
        /// frame, or closes the connection if the token is no good.
        /// </summary>
        public async Task ConnectAsync(string token)
        {
            using (var timeout = CancellationTokenSource.CreateLinkedTokenSource(_cts.Token))
            {
                timeout.CancelAfter(TimeSpan.FromSeconds(10));
                await _socket.ConnectAsync(_uri, timeout.Token);
            }
            // The token goes in the first frame, never in the URL.
            await SendAsync(new ChatRequest { type = "auth", token = token });
            _ = Task.Run(ReceiveLoop);
        }

        /// <summary>Sends one request. Call it from the main thread: it serializes with JsonUtility.</summary>
        public Task SendAsync(ChatRequest request) => SendTextAsync(JsonUtility.ToJson(request));

        async Task SendTextAsync(string json)
        {
            var bytes = new ArraySegment<byte>(Encoding.UTF8.GetBytes(json));
            await _sendLock.WaitAsync(_cts.Token);
            try
            {
                await _socket.SendAsync(bytes, WebSocketMessageType.Text, true, _cts.Token);
            }
            finally
            {
                _sendLock.Release();
            }
        }

        /// <summary>Takes the next thing that arrived, if any. Main thread.</summary>
        public bool TryTake(out ChatIncoming incoming) => _incoming.TryDequeue(out incoming);

        async Task ReceiveLoop()
        {
            var buffer = new byte[16 * 1024];
            var frame = new MemoryStream();
            try
            {
                while (!_cts.IsCancellationRequested)
                {
                    var result = await _socket.ReceiveAsync(new ArraySegment<byte>(buffer), _cts.Token);
                    if (result.MessageType == WebSocketMessageType.Close)
                    {
                        End(_socket.CloseStatus, _socket.CloseStatusDescription);
                        return;
                    }
                    frame.Write(buffer, 0, result.Count);
                    if (frame.Length > MaxFrameBytes)
                    {
                        End(WebSocketCloseStatus.MessageTooBig, "frame too big");
                        return;
                    }
                    if (!result.EndOfMessage)
                        continue;
                    if (result.MessageType == WebSocketMessageType.Text)
                        _incoming.Enqueue(new ChatIncoming { Json = Encoding.UTF8.GetString(frame.GetBuffer(), 0, (int)frame.Length) });
                    frame.SetLength(0);
                }
            }
            catch (Exception e)
            {
                End(null, e is OperationCanceledException ? "closed" : e.Message);
            }
        }

        void End(WebSocketCloseStatus? status, string reason)
        {
            if (Interlocked.Exchange(ref _ended, 1) == 0)
                _incoming.Enqueue(new ChatIncoming { CloseStatus = status, CloseReason = reason });
        }

        /// <summary>Closes the connection politely (best effort) and frees it.</summary>
        public void Dispose()
        {
            if (_socket.State == WebSocketState.Open)
            {
                // Fire and forget: the game shouldn't wait on the network to quit.
                _ = _socket.CloseOutputAsync(WebSocketCloseStatus.NormalClosure, "bye", CancellationToken.None)
                    .ContinueWith(_ => _socket.Dispose());
            }
            else
            {
                _socket.Dispose();
            }
            _cts.Cancel();
        }
    }
}
