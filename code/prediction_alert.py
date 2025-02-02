import cv2
from ultralytics import YOLO
from camera_input import get_rpi_camera_stream
import subprocess  # To run the SMS script

# Load the trained YOLOv8 model
model = YOLO('/home/gari/Desktop/Fin/detect/train/weights/best.pt')

# Use the camera stream or video file as input
# Uncomment one of these lines based on your input source:
# cap = get_rpi_camera_stream()  # Use Raspberry Pi camera stream
cap = cv2.VideoCapture('/home/gari/Desktop/Fin/test_videos/lions_rongai.mp4')  # Use a video file

# Get video properties
width = int(cap.get(cv2.CAP_PROP_FRAME_WIDTH))
height = int(cap.get(cv2.CAP_PROP_FRAME_HEIGHT))
fps = int(cap.get(cv2.CAP_PROP_FPS))

# Define the codec and create VideoWriter for saving output video
fourcc = cv2.VideoWriter_fourcc(*'mp4v')
out = cv2.VideoWriter('/home/gari/Desktop/Fin/test_videos/annotated_output.mp4', fourcc, fps, (width, height))

# Flag to avoid sending multiple SMS for the same detection
sms_sent = False

# Path to your SMS script
sms_script_path = '/twilio_sms_alert.py'

# Process the video frame by frame
while cap.isOpened():
    ret, frame = cap.read()
    if not ret:
        break

    # Perform inference on the frame
    results = model.predict(source=frame, conf=0.5)

    # Check if a lion is detected in the frame
    for box in results[0].boxes:
        if results[0].names[int(box.cls[0])] == 'lion':  
            print("Lion detected!")
            if not sms_sent:  
                # Call the SMS script
                try:
                    subprocess.run(['python3', sms_script_path], check=True)
                    print("SMS script executed successfully.")
                except subprocess.CalledProcessError as e:
                    print(f"Error executing SMS script: {e}")
                sms_sent = True
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
