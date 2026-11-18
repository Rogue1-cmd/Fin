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

## Quickstart

```bash
pip install -r code/requirements.txt

export AT_USERNAME=sandbox            # Africa's Talking username
export AT_API_KEY=your_api_key
export AT_RECIPIENTS="+2547XXXXXXXX"  # comma-separated
export AT_SENDER_ID=                  # optional

python code/prediction_alert.py
```

Set the model path (`runs/detect/train4/weights/best.pt`) and the video or camera input at the top of `prediction_alert.py` first. Alerts are rate-limited to one per 5 minutes.

## Repository

| Path | Contents |
|------|----------|
| `code/` | Inference and alert scripts |
| `runs/detect/train4/` | Final training run: weights, curves, metrics |
| `test_videos/` | Sample lion footage and annotated output |
| `docs/` | Final report, proposal, progress report format |
| `Machine-Is-Learning/` | Training experiments and dataset tooling |

## Notes

- Only lions are detected. Hyenas and other species are future work.
- The report describes SMS via Twilio. The code now uses Africa's Talking.
