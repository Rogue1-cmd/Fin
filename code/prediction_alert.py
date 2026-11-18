"""Detect lions in a video source, send an SMS alert (Africa's Talking) and optionally
stream the annotated feed to the web viewer.

Examples:
    python code/prediction_alert.py --source test_videos/lions_rongai.mp4
    python code/prediction_alert.py --source 0                 # USB / laptop webcam
    python code/prediction_alert.py --source picam --headless  # Raspberry Pi camera
    python code/prediction_alert.py --source picam --headless \\
        --server wss://your-domain/ingest --device-key $DEVICE_KEY
"""
import argparse
import os
import time
from datetime import datetime, timezone

import cv2
from ultralytics import YOLO

from smsalert import send_sms

# Minimum seconds between SMS alerts, so one sighting doesn't send a message per frame
ALERT_COOLDOWN_S = 300


def open_source(source):
    """Open a video file path, a webcam index ("0", "1", ...) or the Pi camera ("picam")."""
    if source == "picam":
        from camera_input import get_rpi_camera_stream
        return get_rpi_camera_stream()
    cap = cv2.VideoCapture(int(source) if source.isdigit() else source)
    if not cap.isOpened():
        raise SystemExit(f"Could not open source: {source}")
    return cap


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--model", default="runs/detect/train4/weights/best.pt", help="path to YOLO weights")
    p.add_argument("--source", default="0", help="video file, webcam index, or 'picam'")
    p.add_argument("--conf", type=float, default=0.15, help="detection confidence threshold (model is under-confident)")
    p.add_argument("--imgsz", type=int, default=640, help="inference size (try 320 on a Raspberry Pi)")
    p.add_argument("--output", help="optional path to save an annotated video (.mp4)")
    p.add_argument("--headless", action="store_true", help="no preview window (for SSH / no display)")
    p.add_argument("--server", default=os.environ.get("STREAM_URL"),
                   help="web viewer ingest URL, e.g. wss://host/ingest (env STREAM_URL)")
    p.add_argument("--device-key", default=os.environ.get("DEVICE_KEY"),
                   help="shared secret for the web viewer (env DEVICE_KEY)")
    p.add_argument("--stream-fps", type=int, default=8, help="frames per second uploaded to the web viewer")
    p.add_argument("--stream-alert-cooldown", type=int, default=300,
                   help="minimum seconds between alerts (snapshot + clip) sent to the web viewer")
    args = p.parse_args()

    stream = None
    if args.server:
        if not args.device_key:
            raise SystemExit("--device-key (or DEVICE_KEY) is required with --server")
        from stream_client import StreamClient
        stream = StreamClient(args.server, args.device_key, fps=args.stream_fps)
        stream.start()

    model = YOLO(args.model)
    cap = open_source(args.source)

    out = None
    if args.output:
        width = int(cap.get(cv2.CAP_PROP_FRAME_WIDTH))
        height = int(cap.get(cv2.CAP_PROP_FRAME_HEIGHT))
        fps = cap.get(cv2.CAP_PROP_FPS) or 30  # some cameras report 0
        out = cv2.VideoWriter(args.output, cv2.VideoWriter_fourcc(*"mp4v"), fps, (width, height))

    last_alert_time = None
    last_stream_alert = None
    try:
        while cap.isOpened():
            ret, frame = cap.read()
            if not ret:
                break

            results = model.predict(source=frame, conf=args.conf, imgsz=args.imgsz, verbose=False)
            result = results[0]
            annotated = result.plot()

            lion_confs = [float(box.conf[0]) for box in result.boxes
                          if result.names[int(box.cls[0])].lower() == "lion"]
            if lion_confs:
                print("Lion detected!")
                now = time.monotonic()
                if last_alert_time is None or now - last_alert_time >= ALERT_COOLDOWN_S:
                    # Only start the cooldown if the SMS was actually sent, so failures are retried
                    if send_sms("lion"):
                        last_alert_time = now
                        if stream:
                            stream.update_status(last_sms=datetime.now(timezone.utc).isoformat())
                if stream and (last_stream_alert is None or now - last_stream_alert >= args.stream_alert_cooldown):
                    stream.push_frame(annotated)
                    if stream.send_alert(annotated, max(lion_confs)):
                        last_stream_alert = now

            if stream:
                stream.push_frame(annotated)
            if out:
                out.write(annotated)
            if not args.headless:
                cv2.imshow("Wildlife detection (q to quit)", annotated)
                if cv2.waitKey(1) & 0xFF == ord("q"):
                    break
    except KeyboardInterrupt:
        pass
    finally:
        if stream:
            stream.update_status(camera_ok=False)
            time.sleep(0.5)  # let the last status go out
            stream.stop()
        cap.release()
        if out:
            out.release()
        cv2.destroyAllWindows()


if __name__ == "__main__":
    main()
