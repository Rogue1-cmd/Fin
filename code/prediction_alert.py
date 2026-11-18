"""Detect lions in a video source and send an SMS alert (Africa's Talking).

Examples:
    python code/prediction_alert.py --source test_videos/lions_rongai.mp4
    python code/prediction_alert.py --source 0                 # USB / laptop webcam
    python code/prediction_alert.py --source picam --headless  # Raspberry Pi camera
"""
import argparse
import time

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
    args = p.parse_args()

    model = YOLO(args.model)
    cap = open_source(args.source)

    out = None
    if args.output:
        width = int(cap.get(cv2.CAP_PROP_FRAME_WIDTH))
        height = int(cap.get(cv2.CAP_PROP_FRAME_HEIGHT))
        fps = cap.get(cv2.CAP_PROP_FPS) or 30  # some cameras report 0
        out = cv2.VideoWriter(args.output, cv2.VideoWriter_fourcc(*"mp4v"), fps, (width, height))

    last_alert_time = None
    try:
        while cap.isOpened():
            ret, frame = cap.read()
            if not ret:
                break

            results = model.predict(source=frame, conf=args.conf, imgsz=args.imgsz, verbose=False)
            result = results[0]

            if any(result.names[int(box.cls[0])].lower() == "lion" for box in result.boxes):
                print("Lion detected!")
                now = time.monotonic()
                if last_alert_time is None or now - last_alert_time >= ALERT_COOLDOWN_S:
                    # Only start the cooldown if the SMS was actually sent, so failures are retried
                    if send_sms("lion"):
                        last_alert_time = now

            annotated = result.plot()
            if out:
                out.write(annotated)
            if not args.headless:
                cv2.imshow("Wildlife detection (q to quit)", annotated)
                if cv2.waitKey(1) & 0xFF == ord("q"):
                    break
    except KeyboardInterrupt:
        pass
    finally:
        cap.release()
        if out:
            out.release()
        cv2.destroyAllWindows()


if __name__ == "__main__":
    main()
