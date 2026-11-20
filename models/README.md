# FaceSign runtime models

FaceSign By uses **SCRFD-10GF + ArcFace ResNet50** for identity recognition. Place the separately authorized weights under `models/by/` following [the installation guide](by/README.md). The full Windows package includes these two files only when they are supplied with a valid local `SHA256SUMS` manifest; the installer verifies them before stopping or replacing an existing service.

## Current default runtime models

The default passive presentation-attack detection backend is the Silent-Face **MiniFASNetV2 + MiniFASNetV1SE ensemble**. YOLOX-Nano supports person detection and tracking.

| File | Role | SHA-256 |
|---|---|---|
| `MiniFASNetV2.onnx` | Passive anti-spoof model, crop scale 2.7 | `b32929adc2d9c34b9486f8c4c7bc97c1b69bc0ea9befefc380e4faae4e463907` |
| `MiniFASNetV1SE.onnx` | Passive anti-spoof model, crop scale 4.0 | `ebab7f90c7833fbccd46d3a555410e78d969db5438e169b6524be444862b3676` |
| `yolox_nano.onnx` | Person detection / tracking | `c789161ed43c8269fcd4e67c67eeeb4e80c622da2eb296a20bc6007bd18a0b7d` |

The two Silent-Face models use 80x80 BGR CHW input with raw 0..255 values, no mean/scale normalization, and 3-class logits. Class 1 is the bona-fide/live class; classes 0 and 2 are attack classes. Face crops follow the original Silent-Face CropImage behavior, and the two live probabilities are averaged before the existing temporal PAD logic.

The Silent-Face ONNX exports are based on Minivision's Silent-Face-Anti-Spoofing project and are Apache-2.0 licensed. YOLOX-Nano is distributed with its upstream license notice.

## Optional legacy A/B backends

`FACESIGN_LIVENESS_BACKEND=mn3` and `FACESIGN_LIVENESS_BACKEND=minifas` remain available for controlled comparisons, but their ONNX files are intentionally not bundled in the current full package. If one of these backends is explicitly selected, FaceSign resolves its corresponding model separately. The normal/default installation uses only the three current runtime models listed above.
