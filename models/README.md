# FaceSign pinned models

The ONNX model binaries are stored once in Git history and are intentionally excluded from normal Windows program artifacts.

Pinned model commit:

`de5287c66e9e37e9f804686bf63f5a0974f68f72`

Files:

- `face_detection_yunet_2023mar.onnx` — YuNet face detection.
- `face_recognition_sface_2021dec.onnx` — SFace face recognition.
- `anti-spoof-mn3.onnx` — passive RGB anti-spoofing classifier used for liveness checks.

Integrity values are listed in `SHA256SUMS`.

The Windows installer reuses valid installed models. A fresh machine downloads the fixed files from the pinned FaceSign commit once and verifies SHA-256 before starting FaceSign.

## anti-spoof-mn3 provenance

`anti-spoof-mn3` is the public Open Model Zoo MobileNetV3 anti-spoof model trained on CelebA-Spoof. The upstream model description reports 128x128 RGB input and a two-class output where class 0 is real and class 1 is spoof.

Source project: `kprokofi/light-weight-face-anti-spoofing`.
Open Model Zoo entry: `models/public/anti-spoof-mn3`.
Upstream project license: MIT. The license text is included in `ANTI_SPOOF_LICENSE.txt`.
