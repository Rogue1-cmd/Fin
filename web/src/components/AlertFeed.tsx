import { Alert } from "../api";
import { ago } from "../time";

export default function AlertFeed({ alerts, freshId }: { alerts: Alert[]; freshId: number | null }) {
  return (
    <section className="card feed">
      <h2>Alerts</h2>
      {alerts.length === 0 && <p className="muted">No lions detected yet.</p>}
      <ul>
        {alerts.map((a) => (
          <li key={a.id} className={a.id === freshId ? "fresh" : ""}>
            <img src={a.snapshot} alt="" loading="lazy" />
            <div>
              <strong>Lion detected</strong>
              <div className="muted">
                {ago(a.ts)} · {(a.confidence * 100).toFixed(0)}% confidence
              </div>
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}
