// A small client for the chat server's HTTP API. Every call sends the
// signed-in user's ID as X-User-ID; errors are thrown with the server's
// own message ({"error": "..."}) and the HTTP status.

export class ApiError extends Error {
  constructor(message, status) {
    super(message);
    this.status = status;
  }
}

export async function api(method, path, { body, userId, headers = {} } = {}) {
  const h = { ...headers };
  if (userId) h["X-User-ID"] = userId;
  if (body !== undefined) h["Content-Type"] = "application/json";

  const res = await fetch(path, {
    method,
    headers: h,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) {
    throw new ApiError(data?.error ?? `${res.status} ${res.statusText}`, res.status);
  }
  return data;
}

// The live feed. EventSource can't send headers, so the user ID goes in the
// URL; the server's Auth middleware accepts ?user= for exactly this reason.
export function watchRoom(roomId, userId) {
  return new EventSource(`/rooms/${roomId}/stream?user=${encodeURIComponent(userId)}`);
}
