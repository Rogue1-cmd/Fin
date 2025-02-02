import cv2
from ultralytics import YOLO
from camera_input import get_rpi_camera_stream
import subprocess

# Loading the trained YOLOv8 model
model = YOLO('/home/gari/Desktop/Fin/detect/train/weights/last.pt')  

# Path to your video file
video_path = '/home/gari/Desktop/Fin/test_videos/lions_rongai.mp4' 
output_path = '/home/gari/Desktop/Fin/test_videos/annotated_output.mp4'


# Open the video file
cap = cv2.VideoCapture(video_path)

#Use camera stream 
#cap = get_rpi_camera_stream()

# Get video properties
width = int(cap.get(cv2.CAP_PROP_FRAME_WIDTH))
height = int(cap.get(cv2.CAP_PROP_FRAME_HEIGHT))
fps = int(cap.get(cv2.CAP_PROP_FPS))

# Define the codec and create VideoWriter for saving output video
fourcc = cv2.VideoWriter_fourcc(*'mp4v')  
out = cv2.VideoWriter(output_path, fourcc, fps, (width, height))

# Process the video frame by frame
while cap.isOpened():
    ret, frame = cap.read()
    if not ret:
        break

    # Perform inference on the frame
    results = model.predict(source=frame, conf=0.5)  

    # Annotate the frame with detection results
    annotated_frame = results[0].plot()

    # Write the annotated frame to the output video
    out.write(annotated_frame)

    # Display the frame with annotations
    cv2.imshow('YOLOv8 Inference', annotated_frame)
    if cv2.waitKey(1) & 0xFF == ord('q'):  
        break

# Release resources
cap.release()
out.release()
cv2.destroyAllWindows()

print(f"Annotated video saved to {output_path}")
