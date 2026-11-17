# FaceSign model upgrade track (By)

The By branch now switches the default face backend to SCRFD-10GF + ArcFace ResNet50. It keeps YOLOX-Nano person tracking and the existing passive RGB PAD engine.

## Runtime implementation

- SCRFD-10GF ONNX detector: 640x640 input, three FPN strides (8/16/32), score/bbox/keypoint decode, NMS, and five-point landmarks.
- ArcFace ResNet50 ONNX recognizer: 112x112 similarity alignment, RGB normalization, 512-D output, L2 normalization.
- ONNX Runtime CPU execution remains the runtime.
- Model backend selection: `--face-model by` (default) or `--face-model legacy`.
- By weights are external and SHA-256 checked. No restricted pretrained weights are bundled or automatically downloaded.
- Optional 106-point and 3D-68 models are recognized in the model manifest, but their inference pipeline is not yet enabled. Gender/age is excluded.

## Feature-space migration

The model version is stored with each face sample. On startup, the selected model is activated. Samples belonging to another feature space are copied to `face_samples_legacy` and deleted from the active table. Student, class, seat, attendance, camera, account, and settings records are not deleted. SFace embeddings cannot be converted to ArcFace embeddings; re-enrollment or authorized source photos are required.

## Licensing

The upstream InsightFace model zoo states that its pretrained model weights are for non-commercial research purposes only. The software code licence does not grant broader rights to the weights. Use only a checkpoint whose exact weight licence covers the intended deployment, or obtain written permission. See `models/by/README.md`.

## Validation

- Unit tests cover SCRFD decode, alignment transforms, vector normalization, model hash checks, and feature-space migration.
- Windows CI builds and runs the legacy smoke tests because the restricted By weights are not included in CI artifacts.
- A real By runtime smoke test still requires the two authorized ONNX files in `models/by`.
