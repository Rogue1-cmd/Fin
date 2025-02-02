from ultralytics import YOLO

model = YOLO("yolov8n.yaml")

# Train the YOLO model
results =model.train(data='config.yaml', epochs=20, imgsz=640)
