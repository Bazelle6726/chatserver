// Quickstart: put this on an empty GameObject in any scene, press Play, and
// chat. It adds a ChatClient and a ChatPanel, logs in with the name and
// password below (creating the account the first time), and joins a lobby.
//
// Run the chat server first:  go run ./cmd/server
// To chat with yourself, build the game and run it next to the Editor with a
// different name.
//
// Unity 2022.1 and newer block plain http:// by default. For this local test,
// set Project Settings > Player > Other Settings > Allow downloads over HTTP
// to "Allowed in development builds".
//
// This logs in with a password, which is only for trying it out. In a real
// game, your backend calls POST /game/sessions and passes the token to
// ChatClient.Connect instead: see the package README.
using System;
using ChatServer.Chat;
using UnityEngine;

public class ChatQuickstart : MonoBehaviour
{
    public string serverUrl = "ws://localhost:8080/ws";
    public string chatName = "player1";
    [Tooltip("8 or more characters. Dev only: real games sign players in through their backend.")]
    public string password = "dev-password";
    public string lobby = "lobby";

    async void Start()
    {
        var chat = gameObject.AddComponent<ChatClient>();
        chat.serverUrl = serverUrl;
        chat.joinOnConnect = new[] { lobby };
        var panel = gameObject.AddComponent<ChatPanel>();

        // Pause your own controls while the player types. Here we just log it.
        panel.TypingChanged += typing => Debug.Log(typing ? "[chat] typing: ignore game keys" : "[chat] done typing");
        chat.Connected += me => Debug.Log($"[chat] connected as {me.name}");
        chat.Joined += (room, history) => Debug.Log($"[chat] joined {room.name} ({history.Length} recent messages)");

        try
        {
            var token = await ChatDevAuth.LoginOrSignUpAsync(ChatDevAuth.HttpUrlFor(serverUrl), chatName, password);
            chat.Connect(token);
        }
        catch (Exception e)
        {
            Debug.LogError($"[chat] {e.Message} (is the server running at {serverUrl}?)");
        }
    }
}
