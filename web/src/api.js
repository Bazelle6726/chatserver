// A small client for the chat server's HTTP API. The session lives in an
// HttpOnly cookie that the server sets when you sign up or log in, and the
// browser sends it with every request by itself, so this code never sees or
// stores the token. Errors are thrown with the server's own message
// ({"error": "..."}) and the HTTP status.

export class ApiError extends Error {
  constructor(message, status) {
    super(message);
    this.status = status;
  }
}

export async function api(method, path, { body, headers = {} } = {}) {
  const h = { ...headers };
  if (body !== undefined) h["Content-Type"] = "application/json";

  const res = await fetch(path, {
    method,
    headers: h,
    credentials: "same-origin", // send the session cookie (the default, made explicit)
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) {
    throw new ApiError(data?.error ?? `${res.status} ${res.statusText}`, res.status);
  }
  return data;
}

// The live feed. EventSource can't send headers, but it does send cookies,
// so the session cookie logs it in.
export function watchRoom(roomId) {
  return new EventSource(`/rooms/${encodeURIComponent(roomId)}/stream`);
}
