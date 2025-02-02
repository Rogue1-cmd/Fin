import cv2

def main():
    # Initialize the camera module
    camera = cv2.VideoCapture(0)  

    # Check if the camera is successfully opened
    if not camera.isOpened():
        print("Error: Could not access the camera.")
        return

    print("Press 'q' to exit the real-time monitoring.")

    while True:
        # Capture frame-by-frame
        ret, frame = camera.read()

        # Check if the frame was successfully captured
        if not ret:
            print("Error: Failed to capture frame.")
            break

        # Display the captured frame
        cv2.imshow("Real-Time Monitoring", frame)

        # Exit the loop when 'q' is pressed
        if cv2.waitKey(1) & 0xFF == ord('q'):
            break

    # Release the camera and close all OpenCV windows
    camera.release()
    cv2.destroyAllWindows()

if __name__ == "__main__":
    main()
