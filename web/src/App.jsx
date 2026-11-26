import { Suspense, startTransition, use, useState } from "react";
import { api } from "./api.js";
import SignIn from "./components/SignIn.jsx";
import RoomList from "./components/RoomList.jsx";
import ChatRoom from "./components/ChatRoom.jsx";
import Moderation from "./components/Moderation.jsx";

// After a reload, the session cookie may still be good: ask who we are.
// A 401 just means not signed in.
const userAtLoad = api("GET", "/me").catch(() => null);

export default function App() {
  const signedInAtLoad = use(userAtLoad);
  const [me, setMe] = useState(signedInAtLoad); // {id, name, role}
  const [room, setRoom] = useState(null); // {id, name, members}
  const [status, setStatus] = useState({ text: signedInAtLoad ? "signed in" : "not signed in", error: false });
  const [joins, setJoins] = useState(0); // bumped after each join, so the room list refreshes its counts

  // In a transition, so the room list keeps showing while it reloads.
  const joined = () => startTransition(() => setJoins((n) => n + 1));
  const say = (text) => setStatus({ text, error: false });
  const fail = (err) => setStatus({ text: err.message ?? String(err), error: true });

  function signedIn(user) {
    setMe(user);
    say("signed in");
  }

  async function signOut() {
    try {
      await api("DELETE", "/sessions"); // ends the session and clears the cookie
    } catch {}
    setMe(null);
    setRoom(null);
    say("not signed in");
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
          {me && (
            <Suspense fallback={<p className="count">Loading rooms…</p>}>
              <RoomList me={me} current={room} refresh={joins} onOpen={setRoom} onError={fail} />
            </Suspense>
          )}
          <Moderation onStatus={say} onError={fail} />
        </aside>

        <ChatRoom key={room?.id ?? "none"} me={me} room={room} onJoined={joined} onStatus={say} onError={fail} />
      </main>
    </>
  );
}
