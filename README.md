# Wildlife Detection and Notification System

Detects lions in a camera feed, sends an SMS alert, and streams the feed to a password-protected web app. Built on YOLOv8 and OpenCV for a Raspberry Pi.

BSc Electronic and Computer Engineering final year project, JKUAT (2025).

**📄 [Read the full report](docs/Project-Final-Report.pdf) · [⬇ Download (PDF)](https://github.com/Rogue1-cmd/Fin/raw/main/docs/Project-Final-Report.pdf)**

## How it works

```
Camera → OpenCV → YOLOv8 lion detection ─┬─▶ SMS alert (Africa's Talking)
                                         └─▶ Web viewer (live video, alerts, clips)
```

The model is YOLOv8n, fine-tuned on lion images from Open Images v6. Trained 20 epochs at 640×640 (`runs/detect/train4`):

| Precision | Recall | mAP50 | mAP50-95 |
|-----------|--------|-------|----------|
| 0.79      | 0.63   | 0.68  | 0.35     |

## Run locally

Needs Python 3.9+.

```bash
git clone --depth 1 https://github.com/Rogue1-cmd/Fin.git && cd Fin
python3 -m venv venv && source venv/bin/activate   # Windows: venv\Scripts\activate
pip install -r code/requirements.txt

python code/prediction_alert.py --source test_videos/lions_rongai.mp4   # sample video
python code/prediction_alert.py --source 0                              # webcam
```

Press `q` to quit. Add `--output result.mp4` to save the annotated video.

## Run on a Raspberry Pi

Raspberry Pi 4B, 64-bit Raspberry Pi OS.

```bash
sudo apt update && sudo apt install -y git python3-venv python3-opencv gstreamer1.0-libcamera
git clone --depth 1 https://github.com/Rogue1-cmd/Fin.git && cd Fin
python3 -m venv --system-site-packages venv && source venv/bin/activate
pip install -r code/requirements.txt
pip uninstall -y opencv-python   # use the apt OpenCV (has Pi camera support)

python code/prediction_alert.py --source picam --headless --imgsz 320
```

USB webcam instead: skip the `pip uninstall` and use `--source 0`.

## Web viewer

Log in from anywhere to watch the live feed, see lion alerts as they happen, and browse saved snapshots and clips.

```
Pi (prediction_alert.py) ──WSS──▶ Go server on a VPS ──HTTPS──▶ browser (React)
```

The Pi connects outward, so no port forwarding is needed. It uploads video only while someone is watching (about 2–4 Mbps).

**1. Deploy** on a VPS with Docker and a domain pointing at it:

```bash
cp .env.example .env     # set DOMAIN, ADMIN_USER, JWT_SECRET, DEVICE_KEY
openssl rand -hex 32     # use for JWT_SECRET, and again for DEVICE_KEY
docker compose build
docker compose run --rm --no-deps --entrypoint hashpw server 'your password'   # → ADMIN_PASSWORD_HASH
docker compose up -d
```

Caddy gets the HTTPS certificate automatically. Open `https://your-domain` and sign in.

**2. Point the Pi at it:**

```bash
python code/prediction_alert.py --source picam --headless --imgsz 320 \
  --server wss://your-domain/ingest --device-key <DEVICE_KEY>
```

(`STREAM_URL` and `DEVICE_KEY` environment variables work too.)

**Storage:** each alert saves a snapshot and a 10-second clip. They are deleted after 14 days or once they exceed 2 GB (`RETENTION_DAYS`, `MAX_MEDIA_MB`; `0` = no limit).

**Develop:** run the server with `COOKIE_SECURE=false` and the variables from `.env.example`, then `cd web && npm run dev`. Tests: `cd server && go test ./...`.

## SMS alerts

Sent through [Africa's Talking](https://africastalking.com). Without these set, detections are printed but no SMS is sent:

```bash
export AT_USERNAME=sandbox            # your Africa's Talking username
export AT_API_KEY=your_api_key
export AT_RECIPIENTS="+2547XXXXXXXX"  # comma-separated
export AT_SENDER_ID=                  # optional
```

At most one SMS every 5 minutes. Never commit your API key.

## Options

| Flag | Default | Purpose |
|------|---------|---------|
| `--source` | `0` | Video file, webcam index, or `picam` |
| `--model` | `runs/detect/train4/weights/best.pt` | Model weights |
| `--conf` | `0.15` | Detection confidence threshold |
| `--imgsz` | `640` | Inference size (`320` is faster on a Pi) |
| `--output` | none | Save annotated video |
| `--headless` | off | No preview window |
| `--server` | none | Web viewer URL (`wss://…/ingest`) |
| `--device-key` | none | Shared secret for the web viewer |
| `--stream-fps` | `8` | Frames per second uploaded |
| `--stream-alert-cooldown` | `300` | Seconds between alerts sent to the web viewer |

## Repository

| Path | Contents |
|------|----------|
| `code/` | Detection, SMS and streaming scripts |
| `server/` | Go web server |
| `web/` | React web app |
| `runs/detect/train4/` | Final training run: weights, curves, metrics |
| `test_videos/` | Sample lion footage |
| `docs/` | Final report, proposal, progress report format |
| `Machine-Is-Learning/` | Training experiments and dataset tooling |

## Known limitations

- **Low detection confidence.** On validation images the weights find a lion in 25 of 35 at `--conf 0.1`, 10 of 35 at 0.25, and none at 0.5. Expect missed lions and some false alarms at the 0.15 default. Retraining with more data and epochs is the main fix.
- Lions only. Hyenas and other species are future work.
- The Pi camera path and a real VPS deployment are untested. The web viewer was tested locally and in Docker.
- The report describes SMS via Twilio. The code now uses Africa's Talking.
