import { useState, useSyncExternalStore } from "react";
import { api } from "../api.js";
import { getRoomFeed } from "../roomFeed.js";

// One open room: joins it, starts the live feed, loads the history, and lets
// you post. App gives it a new key per room, so switching rooms starts fresh.
export default function ChatRoom({ me, room, onJoined, onStatus, onError }) {
  if (!me || !room) {
    return (
      <section>
        <div id="roomTitle">No room open</div>
        <div id="messages">
          <p className="empty">Sign in, then pick or create a room.</p>
        </div>
      </section>
    );
  }
  return <OpenRoom me={me} room={room} onJoined={onJoined} onStatus={onStatus} onError={onError} />;
}

function OpenRoom({ me, room, onJoined, onStatus, onError }) {
  const feed = getRoomFeed(room, { joined: onJoined, status: onStatus, error: onError });
  const { messages, ready } = useSyncExternalStore(feed.subscribe, feed.getSnapshot);
  const [text, setText] = useState("");

  async function send(e) {
    e.preventDefault();
    if (!text.trim()) return;
    try {
      await api("POST", `/rooms/${room.id}/messages`, { body: { text } });
      setText(""); // the live feed shows it, filtered like everyone else's
    } catch (err) {
      onError(err);
    }
  }

  return (
    <section>
      <div id="roomTitle">{room.name}</div>
      <div id="messages">
        {ready && messages.length === 0 && <p className="empty">No messages yet. Say hello!</p>}
        {messages.map((m, i) => (
          <div
            key={m.id}
            ref={i === messages.length - 1 ? scrollIntoView : undefined}
            className={`msg${m.author === me.id ? " mine" : ""}${m.text === "[removed]" ? " removed" : ""}`}
          >
            <div className="who">
              {m.author_name || m.author} · {new Date(m.created_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
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

// Keep the newest message in view. A ref callback on the last message runs
// each time a new message becomes the last one.
function scrollIntoView(el) {
  if (el) el.parentElement.scrollTop = el.parentElement.scrollHeight;
}
