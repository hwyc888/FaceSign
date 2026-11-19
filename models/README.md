# FaceSign runtime models

FaceSign By uses SCRFD-10GF + ArcFace ResNet50 for identity recognition. Place independently authorized weights under `models/by/` following [the installation guide](by/README.md).

## Silent-Face MiniFASNet ensemble (new default)

The default passive PAD backend is the upstream-style **MiniFASNetV2 + MiniFASNetV1SE ensemble**.

- `MiniFASNetV2.onnx`: SHA-256 `b32929adc2d9c34b9486f8c4c7bc97c1b69bc0ea9befefc380e4faae4e463907`, crop scale 2.7.
- `MiniFASNetV1SE.onnx`: SHA-256 `ebab7f90c7833fbccd46d3a555410e78d969db5438e169b6524be444862b3676`, crop scale 4.0.
- Both use 80x80 BGR CHW input with raw 0..255 values, no mean/scale normalization, and 3-class logits. Class 1 is the bona-fide/live class; classes 0 and 2 are attack classes.
- Face crop follows the original Silent-Face CropImage behavior.
- The two model live probabilities are averaged before the existing temporal PAD logic.

The ONNX exports come from yakhyo/face-anti-spoofing and are based on Minivision's Silent-Face-Anti-Spoofing project. The models are Apache-2.0 licensed.

## Legacy backends

- `anti-spoof-mn3.onnx`: legacy MN3 backend. Set `FACESIGN_LIVENESS_BACKEND=mn3` before startup.
- `minifasnet-v2se.onnx`: previous FaceSign A/B model. Set `FACESIGN_LIVENESS_BACKEND=minifas` before startup.

The legacy backends remain available for comparison; the new default is the Silent-Face ensemble.
