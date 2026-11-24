// The chat server's WebSocket protocol, as plain serializable classes for
// Unity's JsonUtility. Field names match the JSON exactly, which is why they
// are snake_case. See internal/api/ws.go in the server for the full protocol.
using System;

namespace ChatServer.Chat
{
    /// <summary>A chat user.</summary>
    [Serializable]
    public class ChatUser
    {
        public string id;
        public string name;
        public string role;
    }

    /// <summary>A chat room.</summary>
    [Serializable]
    public class ChatRoom
    {
        public string id;
        public string name;
        public int members;
        /// <summary>A match room: only players the game added can be in it.</summary>
        public bool @private;
    }

    /// <summary>One chat message.</summary>
    [Serializable]
    public class ChatMessage
    {
        public string id;
        /// <summary>The room's ID.</summary>
        public string room;
        /// <summary>The author's user ID.</summary>
        public string author;
        /// <summary>The author's name when they posted.</summary>
        public string author_name;
        public string kind;
        /// <summary>The text. A message with a banned word arrives as "[removed]".</summary>
        public string text;
        /// <summary>When it was posted, as an RFC 3339 timestamp.</summary>
        public string created_at;
    }

    /// <summary>
    /// A frame the server sends. Every frame has the same fields; which ones
    /// are filled depends on <see cref="type"/>: ready, joined, message, sent,
    /// left, error or pong.
    /// </summary>
    [Serializable]
    public class ChatFrame
    {
        public string type;
        public string @ref;
        public string room_id;
        public ChatUser user;
        public ChatRoom room;
        public ChatMessage message;
        public ChatMessage[] history;
        public string error;
        public string code;
        public string reason;
    }

    /// <summary>A frame the client sends: auth, join, send, leave or ping.</summary>
    [Serializable]
    public class ChatRequest
    {
        public string type;
        public string @ref;
        public string token;
        public string room_name;
        public string room_id;
        public string text;
        public int limit;
    }

    /// <summary>An error the server sent back for one request.</summary>
    public readonly struct ChatError
    {
        /// <summary>invalid, unauthenticated, forbidden, not_found, conflict,
        /// rate_limited, too_large, unavailable, internal, or (from the client
        /// itself) not_connected.</summary>
        public readonly string Code;
        public readonly string Message;
        /// <summary>The ref of the request it answers, if it had one.</summary>
        public readonly string Ref;

        public ChatError(string code, string message, string @ref)
        {
            Code = code;
            Message = message;
            Ref = @ref;
        }

        public override string ToString() => $"{Code}: {Message}";
    }
}
