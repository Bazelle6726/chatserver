import { useState } from "react";
import { api } from "../api.js";

// The admin panel for the banned-word list. It shows how many words there
// are; the words themselves stay folded away, so they never end up on a
// projector by accident.
export default function Moderation({ onStatus, onError }) {
  const [token, setToken] = useState("");
  const [words, setWords] = useState(null); // null = not loaded yet
  const [newWord, setNewWord] = useState("");
  const headers = { "X-Admin-Token": token };

  async function load() {
    try {
      const data = await api("GET", "/admin/banned-words", { headers });
      setWords(data.words);
      onStatus("moderation list loaded");
    } catch (e) {
      onError(e);
    }
  }

  async function ban() {
    try {
      const added = await api("POST", "/admin/banned-words", { body: { word: newWord }, headers });
      setNewWord("");
      await load();
      onStatus(`banned: ${added.word}`);
    } catch (e) {
      onError(e);
    }
  }

  async function unban(word) {
    try {
      await api("DELETE", `/admin/banned-words/${encodeURIComponent(word)}`, { headers });
      await load();
      onStatus(`unbanned: ${word}`);
    } catch (e) {
      onError(e);
    }
  }

  return (
    <details id="admin">
      <summary>Moderation (admins)</summary>
      <div className="row" style={{ marginTop: 8 }}>
        <input id="token" type="password" placeholder="admin token" value={token} onChange={(e) => setToken(e.target.value)} />
        <button className="ghost" id="loadWords" onClick={load}>Load</button>
      </div>
      {words && (
        <>
          <p id="wordCount" className="hint">{words.length} banned words</p>
          <details id="wordList">
            <summary>Show the list</summary>
            <div id="words">
              {words.map((w) => (
                <span className="chip" key={w}>
                  {w}
                  <button title={`unban ${w}`} onClick={() => unban(w)}>×</button>
                </span>
              ))}
            </div>
          </details>
        </>
      )}
      <div className="row">
        <input id="newWord" placeholder="ban a word" value={newWord} onChange={(e) => setNewWord(e.target.value)} />
        <button id="addWord" onClick={ban}>Ban</button>
      </div>
      <p className="hint">Needs the server started with CHAT_ADMIN_TOKEN set.</p>
    </details>
  );
}
