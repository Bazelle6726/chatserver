import { use, useState } from "react";
import { api } from "../api.js";

// Load now, and again after every join (member counts move). The list is a
// cached promise, read with use(): one request per user and join count, so
// a re-render doesn't fetch again, and another user never sees this one's.
let cached = { key: null, rooms: null };
function roomsFor(key, onError) {
  if (cached.key !== key) {
    cached = { key, rooms: api("GET", "/rooms").catch((e) => (onError(e), [])) };
  }
  return cached.rooms;
}

export default function RoomList({ me, current, refresh, onOpen, onError }) {
  const rooms = use(roomsFor(`${me.id}:${refresh}`, onError));
  const [newRoom, setNewRoom] = useState("");

  async function create() {
    try {
      const room = await api("POST", "/rooms", { body: { name: newRoom.trim() } });
      setNewRoom("");
      onOpen(room);
    } catch (e) {
      onError(e);
    }
  }

  return (
    <div>
      <h2>Rooms</h2>
      <ul id="rooms">
        {rooms.length === 0 && <li className="count">No rooms yet</li>}
        {rooms.map((r) => (
          <li key={r.id} className={current?.id === r.id ? "active" : ""} onClick={() => onOpen(r)}>
            <span>{r.name}</span>
            <span className="count">{r.members} in</span>
          </li>
        ))}
      </ul>
      <div className="row" style={{ marginTop: 8 }}>
        <input id="newRoom" placeholder="new room" maxLength={20} value={newRoom} onChange={(e) => setNewRoom(e.target.value)} />
        <button id="createRoom" onClick={create}>Add</button>
      </div>
    </div>
  );
}
