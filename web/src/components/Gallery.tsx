import { useEffect, useState } from "react";
import { Alert } from "../api";

export default function Gallery({ alerts }: { alerts: Alert[] }) {
  const [openId, setOpenId] = useState<number | null>(null);
  const open = alerts.find((a) => a.id === openId);

  useEffect(() => {
    if (openId === null) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpenId(null);
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [openId]);

  return (
    <section className="card">
      <h2>Saved snapshots and clips</h2>
      {alerts.length === 0 && <p className="muted">Nothing saved yet.</p>}
      <div className="grid">
        {alerts.map((a) => (
          <button key={a.id} className="thumb" onClick={() => setOpenId(a.id)}>
            <img src={a.snapshot} alt={`Alert at ${new Date(a.ts).toLocaleString()}`} loading="lazy" />
            <span>{new Date(a.ts).toLocaleString()}</span>
            {a.clip && <span className="tag">▶ clip</span>}
          </button>
        ))}
      </div>

      {open && (
        <div className="modal" role="dialog" aria-modal="true" onClick={() => setOpenId(null)}>
          <div className="modal-body" onClick={(e) => e.stopPropagation()}>
            {open.clip ? (
              <video src={open.clip} poster={open.snapshot} controls autoPlay playsInline />
            ) : (
              <>
                <img src={open.snapshot} alt="Alert snapshot" />
                <p className="muted">Clip is still processing, or unavailable.</p>
              </>
            )}
            <div className="modal-foot">
              <span>{new Date(open.ts).toLocaleString()} · {(open.confidence * 100).toFixed(0)}%</span>
              <a href={open.clip ?? open.snapshot} download>Download</a>
              <button className="ghost" onClick={() => setOpenId(null)}>Close</button>
            </div>
          </div>
        </div>
      )}
    </section>
  );
}
