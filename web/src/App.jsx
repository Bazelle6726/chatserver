import { useCallback, useEffect, useState } from "react";
import { api } from "./api.js";
import SignIn from "./components/SignIn.jsx";
import RoomList from "./components/RoomList.jsx";
import ChatRoom from "./components/ChatRoom.jsx";
import Moderation from "./components/Moderation.jsx";

const SAVED_USER = "chat-user";

export default function App() {
  const [me, setMe] = useState(null); // {id, name, role}
  const [room, setRoom] = useState(null); // {id, name, members}
  const [status, setStatus] = useState({ text: "not signed in", error: false });
  const [joins, setJoins] = useState(0); // bumped after each join, so the room list refreshes its counts
  const joined = useCallback(() => setJoins((n) => n + 1), []);

  // useCallback keeps these the same function between renders. Children use
  // them in effect dependencies, and a new function each render would restart
  // those effects (reconnecting the live feed) every time the status changed.
  const say = useCallback((text) => setStatus({ text, error: false }), []);
  const fail = useCallback((err) => setStatus({ text: err.message ?? String(err), error: true }), []);

  // Sign back in after a reload, if this browser has signed in before.
  useEffect(() => {
    let saved = null;
    try {
      saved = localStorage.getItem(SAVED_USER);
    } catch {}
    if (saved) {
      api("GET", `/users/${encodeURIComponent(saved)}`).then(signedIn).catch(() => {});
    }
  }, []);

  function signedIn(user) {
    setMe(user);
    say("signed in");
    try {
      localStorage.setItem(SAVED_USER, user.id);
    } catch {}
  }

  function signOut() {
    setMe(null);
    setRoom(null);
    say("not signed in");
    try {
      localStorage.removeItem(SAVED_USER);
    } catch {}
  }

  return (
    <>
      <header>
        <h1>
          <span className="go">chat</span>server
        </h1>
        <div id="status" className={status.error ? "error" : ""}>
          {status.text}
        </div>
      </header>

      <main>
        <aside>
          <SignIn me={me} onSignedIn={signedIn} onSignOut={signOut} onError={fail} />
          {me && <RoomList me={me} current={room} refresh={joins} onOpen={setRoom} onError={fail} />}
          <Moderation onStatus={say} onError={fail} />
        </aside>

        <ChatRoom key={room?.id ?? "none"} me={me} room={room} onJoined={joined} onStatus={say} onError={fail} />
      </main>
    </>
  );
}
