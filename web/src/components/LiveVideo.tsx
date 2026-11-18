import { useEffect, useRef, useState } from "react";

const BLANK = "data:image/gif;base64,R0lGODlhAQABAAAAACw=";

/**
 * MJPEG <img>. Browsers keep a detached <img>'s multipart request open, which would leave
 * a phantom viewer on the server (and keep the Pi uploading). Blanking src aborts it.
 */
function Stream({ src, onLoad, onError }: { src: string; onLoad: () => void; onError: () => void }) {
  const ref = useRef<HTMLImageElement>(null);
  useEffect(() => {
    const el = ref.current;
    return () => {
      if (el) el.src = BLANK;
    };
  }, []);
  return <img ref={ref} src={src} alt="Live camera feed" onLoad={onLoad} onError={onError} />;
}

export default function LiveVideo({ online, flash }: { online: boolean; flash: number | null }) {
  const [attempt, setAttempt] = useState(0);
  const [loaded, setLoaded] = useState(false);
  const [alarm, setAlarm] = useState(false);

  // The stream only exists while the camera is online, so it reconnects by itself when the
  // camera returns. `attempt` is only bumped to retry after a stream error.
  useEffect(() => {
    if (!online) setLoaded(false);
  }, [online]);

  // Brief red outline when a new lion alert arrives
  useEffect(() => {
    if (flash === null) return;
    setAlarm(true);
    const t = setTimeout(() => setAlarm(false), 4000);
    return () => clearTimeout(t);
  }, [flash]);

  return (
    <div className={`video card${alarm ? " alarm" : ""}`}>
      {online && (
        <Stream
          key={attempt}
          src={`/stream.mjpg?t=${attempt}`}
          onLoad={() => setLoaded(true)}
          onError={() => setTimeout(() => setAttempt((n) => n + 1), 3000)}
        />
      )}
      {online && loaded && <span className="live-badge">● LIVE</span>}
      {(!online || !loaded) && (
        <div className="overlay">
          {online ? "Connecting to camera…" : "Camera offline"}
          {!online && <small>Waiting for the Raspberry Pi to connect</small>}
        </div>
      )}
    </div>
  );
}
