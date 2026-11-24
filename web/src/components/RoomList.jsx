import { useCallback, useEffect, useState } from "react";
import { api } from "../api.js";

export default function RoomList({ current, refresh, onOpen, onError }) {
  const [rooms, setRooms] = useState([]);
  const [newRoom, setNewRoom] = useState("");

  const load = useCallback(
    () => api("GET", "/rooms").then(setRooms).catch(onError),
    [onError],
  );

  // Load now, and again after every join (member counts move).
  useEffect(() => {
    load();
  }, [load, refresh]);

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
