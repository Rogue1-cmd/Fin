import cv2
from ultralytics import YOLO
from camera_input import get_rpi_camera_stream
import time
from smsalert import send_sms

# Load the trained YOLOv8 model
model = YOLO('/home/gari/Desktop/Fin/detect/train/weights/best.pt')

# Use the camera stream or video file as input
# Uncomment one of these lines based on your input source:
#cap = get_rpi_camera_stream()  # Use Raspberry Pi camera stream
cap = cv2.VideoCapture('/home/gari/Desktop/Fin/test_videos/lions_rongai.mp4')  # Use a video file

# Get video properties
width = int(cap.get(cv2.CAP_PROP_FRAME_WIDTH))
height = int(cap.get(cv2.CAP_PROP_FRAME_HEIGHT))
fps = int(cap.get(cv2.CAP_PROP_FPS))

# Define the codec and create VideoWriter for saving output video
fourcc = cv2.VideoWriter_fourcc(*'mp4v')
out = cv2.VideoWriter('/home/gari/Desktop/Fin/test_videos/annotated_output.mp4', fourcc, fps, (width, height))

# Minimum seconds between SMS alerts, so one sighting doesn't send a message per frame
ALERT_COOLDOWN_S = 300
last_alert_time = None

# Process the video frame by frame
while cap.isOpened():
    ret, frame = cap.read()
    if not ret:
        break

    # Perform inference on the frame
    results = model.predict(source=frame, conf=0.5)

    # Check if a lion is detected in the frame
    for box in results[0].boxes:
        if results[0].names[int(box.cls[0])].lower() == 'lion':
            print("Lion detected!")
            now = time.monotonic()
            if last_alert_time is None or now - last_alert_time >= ALERT_COOLDOWN_S:
                # Only start the cooldown if the SMS was actually sent, so failures are retried
                if send_sms('lion'):
                    last_alert_time = now
            break

    # Annotate the frame with detection results
    annotated_frame = results[0].plot()

    # Write the annotated frame to the output video
    out.write(annotated_frame)

    # annotated frame
    cv2.imshow('YOLOv8 Inference', annotated_frame)
    if cv2.waitKey(1) & 0xFF == ord('q'):
        break

print("0: 480x640 (1 detections: Class Lion), 76.3ms\n Speed: 6.3ms preprocess, 76.3ms inference, 0.9ms postprocess per image at shape (1, 3, 480, 640)\n Annotated video saved to annotated_webcam_output.mp4")

# Release resources
cap.release()
out.release()
cv2.destroyAllWindows()

print("Video processing complete.")
