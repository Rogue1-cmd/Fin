import cv2
from ultralytics import YOLO

# Load the trained YOLOv8 model
model = YOLO('/home/gari/Desktop/Fin/detect/train/weights/last.pt')  #path to trained model

# Open the laptop's webcam (use 0 for the default webcam)
cap = cv2.VideoCapture(0)

# Check if the webcam is opened successfully
if not cap.isOpened():
    print("Error: Could not access the webcam.")
    exit()

# Get video properties
width = int(cap.get(cv2.CAP_PROP_FRAME_WIDTH))
height = int(cap.get(cv2.CAP_PROP_FRAME_HEIGHT))
fps = int(cap.get(cv2.CAP_PROP_FPS))

# Define the codec and create a VideoWriter for saving the output video (optional)
output_path = 'annotated_webcam_output.mp4'
fourcc = cv2.VideoWriter_fourcc(*'mp4v')  
out = cv2.VideoWriter(output_path, fourcc, fps, (width, height))

print("Press 'q' to quit the video stream.")

# Process the webcam video feed frame by frame
while cap.isOpened():
    ret, frame = cap.read()
    if not ret:
        print("Error: Failed to capture a frame.")
        break

    # Perform inference on the frame
    results = model.predict(source=frame, conf=0.5)

    # Annotate the frame with detection results
    annotated_frame = results[0].plot()

    # Display the frame with annotations
    cv2.imshow('YOLOv8 Webcam Inference', annotated_frame)

    # Write the annotated frame to the output video (optional)
    out.write(annotated_frame)

    # Exit the loop when 'q' is pressed
    if cv2.waitKey(1) & 0xFF == ord('q'):
        break

# Release resources
cap.release()
out.release()
cv2.destroyAllWindows()

print(f"Annotated video saved to {output_path}")
