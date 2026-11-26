// Comments for this file are in CODE_NOTES.md under "web/src/roomFeed.js".
import { api, ApiError, watchRoom } from "./api.js";

const feeds = new Map();

const empty = { messages: [], ready: false };

export function getRoomFeed(room, on) {
  let feed = feeds.get(room.id);
  if (!feed) {
    feed = createFeed(room, on);
    feeds.set(room.id, feed);
  }
  return feed;
}

function createFeed(room, on) {
  let snapshot = empty;
  let source = null;
  let run = 0;
  const listeners = new Set();

  const emit = (next) => {
    snapshot = next;
    listeners.forEach((listener) => listener());
  };

  const add = (incoming) => {
    const seen = new Set(snapshot.messages.map((m) => m.id));
    const fresh = incoming.filter((m) => m.room === room.id && !seen.has(m.id));
    if (fresh.length > 0) emit({ ...snapshot, messages: [...snapshot.messages, ...fresh] });
  };

  async function start() {
    const mine = ++run;
    const current = () => mine === run;
    try {
      try {
        await api("POST", `/rooms/${room.id}/members`);
      } catch (e) {
        if (!(e instanceof ApiError && e.status === 409)) throw e;
      }
      if (!current()) return;
      on.joined();

      source = watchRoom(room.id);
      source.addEventListener("message", (ev) => add([JSON.parse(ev.data)]));
      source.onopen = () => on.status(`live in ${room.name}`);
      source.onerror = () => on.error(new Error("live feed disconnected: reconnecting…"));

      const history = await api("GET", `/rooms/${room.id}/messages?limit=50`);
      if (!current()) return;
      add(history);
      emit({ ...snapshot, ready: true });
    } catch (e) {
      on.error(e);
    }
  }

  function stop() {
    run++;
    source?.close();
    source = null;
    snapshot = empty;
  }

  return {
    subscribe(listener) {
      listeners.add(listener);
      if (listeners.size === 1) start();
      return () => {
        listeners.delete(listener);
        if (listeners.size === 0) stop();
      };
    },
    getSnapshot: () => snapshot,
  };
}
