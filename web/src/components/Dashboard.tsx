import { useCallback, useEffect, useState } from "react";
import { Alert, ServerEvent, Status, api } from "../api";
import LiveVideo from "./LiveVideo";
import StatusBar from "./StatusBar";
import AlertFeed from "./AlertFeed";
import Gallery from "./Gallery";

const OFFLINE: Status = {
  online: false, viewers: 0, fps: 0, camera_ok: false, last_sms: null, last_alert: null,
};

export default function Dashboard({ user, onLogout }: { user: string; onLogout: () => void }) {
  const [status, setStatus] = useState<Status>(OFFLINE);
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [freshId, setFreshId] = useState<number | null>(null);
  const [live, setLive] = useState(false);

  // Initial history
  useEffect(() => {
    api.alerts().then(setAlerts).catch(() => {});
  }, []);

  // Live updates over Server-Sent Events
  useEffect(() => {
    const es = new EventSource("/api/events");
    es.onopen = () => setLive(true);
    es.onmessage = (m) => {
      const ev = JSON.parse(m.data) as ServerEvent;
      if (ev.type === "status") setStatus(ev.status);
      if (ev.type === "alert") {
        setAlerts((a) => [ev.alert, ...a.filter((x) => x.id !== ev.alert.id)]);
        setFreshId(ev.alert.id);
      }
      if (ev.type === "alert_removed") {
        setAlerts((a) => a.filter((x) => !ev.ids.includes(x.id)));
      }
      if (ev.type === "alert_update") {
        setAlerts((a) => a.map((x) => (x.id === ev.alert.id ? ev.alert : x)));
      }
    };
    es.onerror = () => {
      setLive(false);
      // EventSource retries by itself; if the session expired it stays closed.
      if (es.readyState === EventSource.CLOSED) {
        api.me().catch(() => onLogout());
      }
    };
    return () => es.close();
  }, [onLogout]);

  const logout = useCallback(async () => {
    await api.logout().catch(() => {});
    onLogout();
  }, [onLogout]);

  return (
    <div className="app">
      <header>
        <h1>Wildlife Monitor</h1>
        <div className="header-right">
          <span className="muted">{user}</span>
          <button className="ghost" onClick={logout}>Sign out</button>
        </div>
      </header>

      <StatusBar status={status} connected={live} />

      <main>
        <section className="video-col">
          <LiveVideo online={status.online} flash={freshId} />
          <Gallery alerts={alerts} />
        </section>
        <aside>
          <AlertFeed alerts={alerts} freshId={freshId} />
        </aside>
      </main>
    </div>
  );
}
