// ChatDevAuth gets a session token with a chat name and password, straight
// from the chat server. It's for trying the chat out before your game
// backend is wired up.
//
// In a shipped game, don't use it: your game backend should sign players in
// with POST /game/sessions (it holds the game key) and hand the token to the
// client. Then players need no chat password, and can't pick someone else's
// chat name.
using System;
using System.Text;
using System.Threading.Tasks;
using UnityEngine;
using UnityEngine.Networking;

namespace ChatServer.Chat
{
    public static class ChatDevAuth
    {
        [Serializable]
        class Credentials
        {
            public string name;
            public string password;
        }

        [Serializable]
        class AuthReply
        {
            public ChatUser user;
            public string token;
            public string error;
        }

        /// <summary>
        /// Logs in as name, creating the account first if it doesn't exist,
        /// and returns a session token for ChatClient.Connect. httpUrl is the
        /// server's HTTP address, e.g. http://localhost:8080 (see HttpUrlFor).
        /// Passwords are 8-72 characters.
        /// </summary>
        public static async Task<string> LoginOrSignUpAsync(string httpUrl, string name, string password)
        {
            var (status, reply) = await PostAsync(httpUrl + "/sessions", name, password);
            if (status == 401)
                (status, reply) = await PostAsync(httpUrl + "/users", name, password);
            if (status < 200 || status >= 300 || string.IsNullOrEmpty(reply?.token))
                throw new Exception($"chat login failed ({status}): {reply?.error ?? "no reply"}");
            return reply.token;
        }

        /// <summary>The HTTP address for a WebSocket address: ws://host:8080/ws -> http://host:8080.</summary>
        public static string HttpUrlFor(string webSocketUrl)
        {
            var b = new UriBuilder(webSocketUrl);
            b.Scheme = b.Scheme == "wss" ? "https" : "http";
            b.Path = "";
            return b.Uri.GetLeftPart(UriPartial.Authority);
        }

        static async Task<(long, AuthReply)> PostAsync(string url, string name, string password)
        {
            var body = Encoding.UTF8.GetBytes(JsonUtility.ToJson(new Credentials { name = name, password = password }));
            using (var req = new UnityWebRequest(url, "POST"))
            {
                req.uploadHandler = new UploadHandlerRaw(body);
                req.downloadHandler = new DownloadHandlerBuffer();
                req.SetRequestHeader("Content-Type", "application/json");
                var op = req.SendWebRequest();
                while (!op.isDone)
                    await Task.Yield();
                var text = req.downloadHandler.text;
                AuthReply reply = null;
                if (!string.IsNullOrEmpty(text))
                {
                    try
                    {
                        reply = JsonUtility.FromJson<AuthReply>(text);
                    }
                    catch (Exception)
                    {
                        reply = new AuthReply { error = req.error ?? text };
                    }
                }
                if (reply == null && !string.IsNullOrEmpty(req.error))
                    reply = new AuthReply { error = req.error };
                return (req.responseCode, reply);
            }
        }
    }
}
