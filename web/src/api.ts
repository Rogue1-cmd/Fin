export type Alert = {
  id: number;
  ts: string;
  confidence: number;
  snapshot: string;
  clip?: string;
};

export type Status = {
  online: boolean;
  viewers: number;
  fps: number;
  camera_ok: boolean;
  last_sms: string | null;
  last_alert: string | null;
};

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, { credentials: "same-origin", ...init });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new ApiError(res.status, body.error ?? res.statusText);
  return body as T;
}

export const api = {
  me: () => request<{ user: string }>("/api/me"),
  login: (username: string, password: string) =>
    request<{ user: string }>("/api/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password }),
    }),
  logout: () => request<unknown>("/api/logout", { method: "POST" }),
  alerts: () => request<Alert[]>("/api/alerts?limit=50"),
  status: () => request<Status>("/api/status"),
};

export type ServerEvent =
  | { type: "status"; status: Status }
  | { type: "alert"; alert: Alert }
  | { type: "alert_update"; alert: Alert }
  | { type: "alert_removed"; ids: number[] };
