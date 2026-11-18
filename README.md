# Wildlife Detection and Notification System

Computer vision system that detects lions in a camera feed and sends an SMS alert to residents and authorities. Built on YOLOv8 and OpenCV, targeting a Raspberry Pi with a camera module.

BSc Electronic and Computer Engineering final year project, JKUAT (2025).

**📄 [Read the full report](docs/Project-Final-Report.pdf) · [⬇ Download (PDF)](https://github.com/Rogue1-cmd/Fin/raw/main/docs/Project-Final-Report.pdf)**

## How it works

```
Camera / video → OpenCV frames → YOLOv8 lion detection → SMS alert (Africa's Talking)
```

The model is YOLOv8n, fine-tuned on lion images from Open Images v6.

## Results

Trained 20 epochs at 640×640, batch 16 (`runs/detect/train4`):

| Precision | Recall | mAP50 | mAP50-95 |
|-----------|--------|-------|----------|
| 0.79      | 0.63   | 0.68  | 0.35     |

Limited by training compute. See the report for details and recommendations.

## Run locally (Linux / macOS / Windows)

Needs Python 3.9+.

```bash
git clone --depth 1 https://github.com/Rogue1-cmd/Fin.git && cd Fin
python3 -m venv venv && source venv/bin/activate   # Windows: venv\Scripts\activate
pip install -r code/requirements.txt

# Try it on a sample video (no SMS needed)
python code/prediction_alert.py --source test_videos/lions_rongai.mp4

# Or use a laptop / USB webcam
python code/prediction_alert.py --source 0
```

A preview window opens; press `q` to quit. Add `--output result.mp4` to save the annotated video.

## Run on a Raspberry Pi

Target: Raspberry Pi 4B, 64-bit Raspberry Pi OS. Inference is slow on a Pi, so use a smaller image size.

```bash
sudo apt update && sudo apt install -y git python3-venv python3-opencv gstreamer1.0-libcamera
git clone --depth 1 https://github.com/Rogue1-cmd/Fin.git && cd Fin
python3 -m venv --system-site-packages venv && source venv/bin/activate
pip install -r code/requirements.txt
pip uninstall -y opencv-python   # use the apt OpenCV, which has Pi camera (GStreamer) support

# Pi camera module, no monitor attached
python code/prediction_alert.py --source picam --headless --imgsz 320
```

Using a USB webcam instead? Skip the `pip uninstall` and use `--source 0`. Connect over SSH and add `--headless` when there is no display.

## SMS alerts

Alerts are sent through [Africa's Talking](https://africastalking.com). Set these before running, otherwise detections are printed but no SMS is sent:

```bash
export AT_USERNAME=sandbox            # your Africa's Talking username
export AT_API_KEY=your_api_key
export AT_RECIPIENTS="+2547XXXXXXXX"  # comma-separated
export AT_SENDER_ID=                  # optional
```

Alerts are limited to one every 5 minutes. Never commit your API key.

## Options

| Flag | Default | Purpose |
|------|---------|---------|
| `--source` | `0` | Video file, webcam index, or `picam` |
| `--model` | `runs/detect/train4/weights/best.pt` | Model weights |
| `--conf` | `0.15` | Detection confidence threshold |
| `--imgsz` | `640` | Inference size (`320` is faster on a Pi) |
| `--output` | none | Save annotated video |
| `--headless` | off | No preview window |

## Repository

| Path | Contents |
|------|----------|
| `code/` | Inference and alert scripts |
| `runs/detect/train4/` | Final training run: weights, curves, metrics |
| `test_videos/` | Sample lion footage and annotated output |
| `docs/` | Final report, proposal, progress report format |
| `Machine-Is-Learning/` | Training experiments and dataset tooling |

## Known limitations

- **Weak detection confidence.** The included weights score lions low: on validation images they detect 25 of 35 at `--conf 0.1`, 10 of 35 at 0.25 and none at 0.5. The default is 0.15, so expect missed lions and some false alarms. Retraining with more data and epochs is the main improvement.
- Only lions are detected. Hyenas and other species are future work.
- The Pi camera path has not been tested on hardware in this repo's current state.
- The report describes SMS via Twilio. The code now uses Africa's Talking.
