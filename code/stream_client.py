"""Pushes annotated frames and lion alerts from the Pi to the web server.

Protocol (WebSocket, device key sent as the X-Device-Key header):
  Pi -> server  binary: 1 flag byte + JPEG   (1 = live frame, 2 = pre-roll for clips)
  Pi -> server  text:   {"type":"alert", "confidence":..., "snapshot":"<base64 jpeg>"}
                        {"type":"status", "fps":..., "camera_ok":..., "last_sms":...}
  server -> Pi  text:   {"viewers": n}

Frames are only uploaded while someone is watching (or while recording an alert clip),
so an idle camera uses almost no bandwidth.
"""
import base64
import collections
import json
import queue
import threading
import time

import cv2
import websocket  # pip install websocket-client

FLAG_LIVE = 1
FLAG_PREROLL = 2
STATUS_INTERVAL_S = 10


class StreamClient:
    def __init__(self, url, device_key, fps=8, quality=70, preroll_s=3, postroll_s=7):
        self.url = url
        self.device_key = device_key
        self.interval = 1.0 / fps
        self.fps = fps
        self.quality = quality
        self.postroll_s = postroll_s

        self.connected = False
        self.viewers = 0

        # Recent encoded frames: [jpeg_bytes, already_sent_live]
        self._buffer = collections.deque(maxlen=max(1, int(fps * preroll_s)))
        self._last_push = 0.0
        self._record_until = 0.0
        self._frames_pushed = 0
        self._fps_window_start = time.monotonic()
        self._measured_fps = 0.0

        self._camera_ok = True
        self._last_sms = None
        self._frames_q = queue.Queue(maxsize=16)  # live frames: dropped when the link is slow
        self._ctrl_q = queue.Queue(maxsize=64)  # alerts / pre-roll: kept
        self._stop = threading.Event()
        self._ws = None
        self._threads = [
            threading.Thread(target=self._run_ws, daemon=True),
            threading.Thread(target=self._run_sender, daemon=True),
        ]

    # ---- public API (called from the detection loop) ----
    def start(self):
        for t in self._threads:
            t.start()

    def stop(self):
        self._stop.set()
        if self._ws:
            self._ws.close()

    def update_status(self, camera_ok=None, last_sms=None):
        if camera_ok is not None:
            self._camera_ok = camera_ok
        if last_sms is not None:
            self._last_sms = last_sms

    def push_frame(self, frame_bgr):
        """Call once per processed frame with the annotated image."""
        now = time.monotonic()
        if now - self._last_push < self.interval:
            return
        self._last_push = now
        self._frames_pushed += 1
        if now - self._fps_window_start >= 5:
            self._measured_fps = self._frames_pushed / (now - self._fps_window_start)
            self._frames_pushed = 0
            self._fps_window_start = now

        jpeg = self._encode(frame_bgr)
        if jpeg is None:
            return
        entry = [jpeg, False]
        self._buffer.append(entry)
        if self.connected and (self.viewers > 0 or now < self._record_until):
            try:
                self._frames_q.put_nowait(bytes([FLAG_LIVE]) + jpeg)
                entry[1] = True
            except queue.Full:
                pass  # link is slow: skip this frame

    def send_alert(self, frame_bgr, confidence):
        """Send a snapshot now, the buffered lead-up for the clip, and keep streaming for the post-roll."""
        jpeg = self._encode(frame_bgr)
        if jpeg is None or not self.connected:
            return False
        try:
            for entry in list(self._buffer):
                if not entry[1]:
                    self._ctrl_q.put_nowait((FLAG_PREROLL, bytes([FLAG_PREROLL]) + entry[0]))
                    entry[1] = True
            msg = json.dumps({
                "type": "alert",
                "confidence": round(float(confidence), 3),
                "snapshot": base64.b64encode(jpeg).decode(),
            })
            self._ctrl_q.put_nowait((None, msg))
        except queue.Full:
            return False
        self._record_until = time.monotonic() + self.postroll_s
        return True

    # ---- internals ----
    def _encode(self, frame):
        ok, buf = cv2.imencode(".jpg", frame, [cv2.IMWRITE_JPEG_QUALITY, self.quality])
        return buf.tobytes() if ok else None

    def _on_open(self, ws):
        self.connected = True
        self._backoff = 1
        print("Stream: connected to server")

    def _on_message(self, ws, message):
        try:
            self.viewers = int(json.loads(message).get("viewers", 0))
        except (ValueError, TypeError):
            pass

    def _on_close(self, ws, *args):
        self.connected = False
        self.viewers = 0

    def _on_error(self, ws, error):
        print(f"Stream: {error}")

    def _run_ws(self):
        self._backoff = 1
        while not self._stop.is_set():
            self._ws = websocket.WebSocketApp(
                self.url,
                header=[f"X-Device-Key: {self.device_key}"],
                on_open=self._on_open,
                on_message=self._on_message,
                on_close=self._on_close,
                on_error=self._on_error,
            )
            self._ws.run_forever(ping_interval=20, ping_timeout=10)
            self.connected = False
            self.viewers = 0
            if self._stop.wait(self._backoff):
                return
            self._backoff = min(self._backoff * 2, 30)

    def _send(self, data, binary):
        ws = self._ws
        if ws is None or not self.connected:
            return
        try:
            ws.send(data, websocket.ABNF.OPCODE_BINARY if binary else websocket.ABNF.OPCODE_TEXT)
        except (websocket.WebSocketException, OSError):
            pass  # the reader thread will notice and reconnect

    def _run_sender(self):
        next_status = 0.0
        while not self._stop.is_set():
            if not self.connected:
                self._drain(self._frames_q)  # don't send stale frames after reconnect
                self._stop.wait(0.2)
                continue
            now = time.monotonic()
            if now >= next_status:
                next_status = now + STATUS_INTERVAL_S
                self._send(json.dumps({
                    "type": "status",
                    "fps": round(self._measured_fps, 1),
                    "camera_ok": self._camera_ok,
                    "last_sms": self._last_sms,
                }), binary=False)
            # alerts and pre-roll first, then live frames
            try:
                flag, payload = self._ctrl_q.get_nowait()
                self._send(payload, binary=flag is not None)
                continue
            except queue.Empty:
                pass
            try:
                self._send(self._frames_q.get(timeout=0.2), binary=True)
            except queue.Empty:
                pass

    @staticmethod
    def _drain(q):
        try:
            while True:
                q.get_nowait()
        except queue.Empty:
            pass
