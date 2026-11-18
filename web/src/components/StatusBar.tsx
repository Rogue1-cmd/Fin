import { Status } from "../api";
import { ago } from "../time";

function Pill({ ok, label, value }: { ok?: boolean; label: string; value: string }) {
  return (
    <div className="pill">
      <span className={`dot ${ok === undefined ? "" : ok ? "ok" : "bad"}`} />
      <span className="muted">{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

export default function StatusBar({ status, connected }: { status: Status; connected: boolean }) {
  return (
    <div className="statusbar" aria-label="System status">
      <Pill ok={status.online} label="Camera" value={status.online ? (status.camera_ok ? "Online" : "No video") : "Offline"} />
      <Pill label="FPS" value={status.online ? status.fps.toFixed(1) : "–"} />
      <Pill label="Viewers" value={String(status.viewers)} />
      <Pill label="Last SMS" value={ago(status.last_sms)} />
      <Pill label="Last alert" value={ago(status.last_alert)} />
      <Pill ok={connected} label="Updates" value={connected ? "Live" : "Reconnecting"} />
    </div>
  );
}
