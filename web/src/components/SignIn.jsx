import { useState } from "react";
import { api } from "../api.js";

// Sign up or log in with a name and password. Either way the server sets the
// session cookie, so all this component does with the reply is show who you
// are.
export default function SignIn({ me, onSignedIn, onSignOut, onError }) {
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");

  if (me) {
    return (
      <div>
        <h2>You</h2>
        <div id="me">
          Signed in as <b>{me.name}</b>
        </div>
        <button className="ghost" onClick={onSignOut} style={{ marginTop: 6 }}>
          Sign out
        </button>
      </div>
    );
  }

  const send = (path) => (e) => {
    e.preventDefault();
    api("POST", path, { body: { name: name.trim(), password } })
      .then((res) => {
        setPassword("");
        onSignedIn(res.user);
      })
      .catch(onError);
  };

  return (
    <form onSubmit={send("/sessions")}>
      <h2>You</h2>
      <input id="name" placeholder="name, e.g. alice" maxLength={20} autoComplete="username" value={name} onChange={(e) => setName(e.target.value)} />
      <input
        id="password"
        type="password"
        placeholder="password (8+ characters)"
        maxLength={72}
        autoComplete="current-password"
        value={password}
        onChange={(e) => setPassword(e.target.value)}
        style={{ marginTop: 6 }}
      />
      <div className="row" style={{ marginTop: 6 }}>
        <button id="signin">Log in</button>
        <button type="button" className="ghost" id="signup" onClick={send("/users")}>
          Create account
        </button>
      </div>
    </form>
  );
}
