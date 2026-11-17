# FaceSign runtime models

The current FaceSign branch uses SCRFD-10GF + ArcFace ResNet50 as its only face-recognition backend. Install those two authorized ONNX files separately under `models/by/`; see [models/by/README.md](by/README.md).

This repository packages only the support models required by the application:
- `anti-spoof-mn3.onnx`: passive RGB presentation-attack screening.
- `yolox_nano.onnx`: person detection input for tracking.

The YuNet and SFace model files have been removed from this branch. No legacy face detector or recognizer is available.
