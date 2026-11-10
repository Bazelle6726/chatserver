import { useCallback, useRef, useState } from "react";
import { api } from "./api.js";

// Messages only carry the author's ID, so look names up once and remember
// them. Returns nameOf(id): the name if known, else the ID (and fetches it).
export function useUserNames() {
  const [names, setNames] = useState({});
  const asked = useRef(new Set());

  return useCallback(
    (id) => {
      if (names[id]) return names[id];
      if (!asked.current.has(id)) {
        asked.current.add(id);
        api("GET", `/users/${encodeURIComponent(id)}`)
          .then((u) => setNames((n) => ({ ...n, [id]: u.name })))
          .catch(() => {});
      }
      return id;
    },
    [names],
  );
}
