import { useState } from "react";
import { api } from "../api.js";

export default function SignIn({ me, onSignedIn, onSignOut, onError }) {
  const [name, setName] = useState("");
  const [existing, setExisting] = useState("");

  if (me) {
    return (
      <div>
        <h2>You</h2>
        <div id="me">
          Signed in as <b>{me.name}</b> ({me.id})
        </div>
        <button className="ghost" onClick={onSignOut} style={{ marginTop: 6 }}>
          Sign out
        </button>
      </div>
    );
  }

  const create = () => api("POST", "/users", { body: { name: name.trim() } }).then(onSignedIn).catch(onError);
  const useId = () => api("GET", `/users/${encodeURIComponent(existing.trim())}`).then(onSignedIn).catch(onError);

  return (
    <div>
      <h2>You</h2>
      <div className="row">
        <input id="name" placeholder="new name, e.g. alice" maxLength={20} value={name} onChange={(e) => setName(e.target.value)} />
        <button id="signup" onClick={create}>Create</button>
      </div>
      <div className="row" style={{ marginTop: 6 }}>
        <input id="existing" placeholder="or your ID, e.g. u-1" value={existing} onChange={(e) => setExisting(e.target.value)} />
        <button className="ghost" id="signin" onClick={useId}>Use</button>
      </div>
    </div>
  );
}
