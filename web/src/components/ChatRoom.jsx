import { useEffect, useRef, useState } from "react";
import { api, ApiError, watchRoom } from "../api.js";
import { useUserNames } from "../useUserNames.js";

// One open room: joins it, starts the live feed, loads the history, and lets
// you post. App gives it a new key per room, so switching rooms starts fresh.
export default function ChatRoom({ me, room, onJoined, onStatus, onError }) {
  const [messages, setMessages] = useState([]);
  const [ready, setReady] = useState(false);
  const [text, setText] = useState("");
  const nameOf = useUserNames();
  const list = useRef(null);

  useEffect(() => {
    if (!me || !room) return;
    let source = null;
    let cancelled = false;

    // Add messages, skipping any we already have (by ID), oldest first.
    const add = (incoming) =>
      setMessages((old) => {
        const seen = new Set(old.map((m) => m.id));
        return [...old, ...incoming.filter((m) => m.room === room.id && !seen.has(m.id))];
      });

    (async () => {
      try {
        try {
          await api("POST", `/rooms/${room.id}/members`, { userId: me.id });
        } catch (e) {
          if (!(e instanceof ApiError && e.status === 409)) throw e; // 409 = already a member
        }
        if (cancelled) return;
        onJoined();

        // Watch BEFORE loading the history, so nothing posted in between is lost.
        source = watchRoom(room.id, me.id);
        source.addEventListener("message", (ev) => add([JSON.parse(ev.data)]));
        source.onopen = () => onStatus(`live in ${room.name}`);
        source.onerror = () => onError(new Error("live feed disconnected: reconnecting…"));

        add(await api("GET", `/rooms/${room.id}/messages?limit=50`, { userId: me.id }));
        if (!cancelled) setReady(true);
      } catch (e) {
        onError(e);
      }
    })();

    // Cleanup: runs when the room changes or the component goes away.
    return () => {
      cancelled = true;
      source?.close();
    };
  }, [me, room, onJoined, onStatus, onError]);

  // Keep the newest message in view.
  useEffect(() => {
    if (list.current) list.current.scrollTop = list.current.scrollHeight;
  }, [messages]);

  async function send(e) {
    e.preventDefault();
    if (!text.trim()) return;
    try {
      await api("POST", `/rooms/${room.id}/messages`, { body: { text }, userId: me.id });
      setText(""); // the live feed shows it, filtered like everyone else's
    } catch (err) {
      onError(err);
    }
  }

  if (!room) {
    return (
      <section>
        <div id="roomTitle">No room open</div>
        <div id="messages">
          <p className="empty">Sign in, then pick or create a room.</p>
        </div>
      </section>
    );
  }

  return (
    <section>
      <div id="roomTitle">{room.name}</div>
      <div id="messages" ref={list}>
        {ready && messages.length === 0 && <p className="empty">No messages yet. Say hello!</p>}
        {messages.map((m) => (
          <div key={m.id} className={`msg${m.author === me.id ? " mine" : ""}${m.text === "[removed]" ? " removed" : ""}`}>
            <div className="who">
              {nameOf(m.author)} · {new Date(m.created_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
            </div>
            <div className="text">{m.text}</div>
          </div>
        ))}
      </div>
      <form id="post" onSubmit={send}>
        <input id="text" placeholder="Write a message" maxLength={500} autoComplete="off" disabled={!ready} value={text} onChange={(e) => setText(e.target.value)} autoFocus />
        <button id="send" disabled={!ready}>Send</button>
      </form>
    </section>
  );
}
