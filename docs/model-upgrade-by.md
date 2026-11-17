# FaceSign model upgrade (By)

The By branch replaces YuNet + SFace with one supported face-recognition pipeline: SCRFD-10GF detection and ArcFace ResNet50 embeddings (512 dimensions). There is no legacy model backend or runtime fallback.

## Runtime

- SCRFD-10GF ONNX: 640x640 RGB input, stride 8/16/32 output decoding, bounding boxes, five facial landmarks, NMS.
- ArcFace ResNet50 ONNX: five-point similarity alignment, 112x112 BGR input, 512-D output, L2 normalization.
- ONNX Runtime CPU.
- YOLOX-Nano person detection/tracking remains.
- anti-spoof-mn3 passive PAD and existing multi-frame voting remain.

## Weights

The SCRFD and ArcFace weights are not included in this repository or CI artifacts. Place authorized files in `models/by/` and generate the local `SHA256SUMS` manifest using the commands in `models/by/README.md`. The checksum verifies local integrity, not publisher authenticity.

The upstream InsightFace model zoo marks its pretrained weights for non-commercial research. Obtain an appropriate licence or use another checkpoint whose exact terms permit the intended school deployment. Do not assume the source-code licence grants rights to pretrained weights.

## Feature-space migration

Every face sample carries a model-version tag. On startup, the app archives samples from older model spaces in `face_samples_archive` and deletes them from the active `face_samples` table. No old inference backend is retained. Student profiles, class/seat assignments, attendance, cameras, user accounts, and settings are preserved. Students must be re-enrolled or imported from authorized source photos to create new ArcFace vectors.

## CI

Windows CI runs Go tests/vet and produces Windows x64 packages. Because restricted or user-provided face weights are not redistributed, CI does not run camera/model inference smoke tests unless `FACESIGN_RUN_MODEL_SMOKE=true` and authorized model files have been made available to the runner.
