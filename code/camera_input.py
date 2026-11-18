import cv2

def get_rpi_camera_stream():
    camera_stream = "libcamerasrc ! video/x-raw, width=640, height=480, framerate=30/1 ! videoconvert ! appsink"
    cap = cv2.VideoCapture(camera_stream, cv2.CAP_GSTREAMER)

    if not cap.isOpened():
        raise Exception("Error: Could not open the Raspberry Pi camera stream.")

    return cap
