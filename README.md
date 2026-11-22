# FaceSign

FaceSign is a compact school face-attendance application written in Go for Windows.

## Face-recognition stack

This branch has one supported face-recognition pipeline:

- **SCRFD-10GF**: face detection and five facial landmarks.
- **ArcFace ResNet50**: 512-dimensional identity embeddings.
- **Silent-Face MiniFASNetV2 + MiniFASNetV1SE**: default passive RGB presentation-attack screening; optional legacy MN3 and MiniFAS A/B backends are not bundled.
- **YOLOX-Nano**: person detection used by the person-tracking workflow.
- **ONNX Runtime CPU**: inference without a GPU requirement.

YuNet and SFace have been removed from this branch. There is no legacy face-model backend or runtime fallback.

The authorized SCRFD and ArcFace ONNX files belong under `models/by/` with a matching `SHA256SUMS` manifest. The Windows full-package workflow includes them when the three authorized input files are supplied; it never substitutes a different checkpoint. The upstream InsightFace model-zoo weights have restrictive terms, so do not redistribute them unless the intended school-attendance use and package distribution are covered by your licence or written permission.

## Features

- Student and class management, including seat assignments.
- Browser camera enrollment and batch photo import from multiple image files or a ZIP archive, with a default class parameter and automatic class-folder detection.
- Face recognition and daily check-in.
- One check-in record per student per day.
- Attendance history and class seat board.
- Embedded responsive web UI.
- Local SQLite database.
- Portable Windows operation; optional startup registration through FaceSignManager.
- USB and RTSP camera support through the bundled FFmpeg runtime.

## Windows packages

GitHub Actions publishes:

- `facesign-windows-amd64-full`: application, native runtime, FFmpeg, the current Silent-Face ensemble, YOLOX-Nano, and the By face models when the authorized SCRFD/ArcFace files and checksum manifest are supplied to the build.
- `facesign-windows-amd64-lite`: lightweight application upgrade package without model files.
- `facesign-windows-amd64-full-no-ffmpeg`: full package without the FFmpeg runtime.
- `facesign-windows-amd64-no-models-no-ffmpeg`: minimal application package.
- `facesign-camera-agent-windows-amd64`: remote camera agent.

The full package is self-contained for identity recognition only when `models/by/scrfd_10g.onnx`, `models/by/arcface_w600k_r50.onnx`, and `models/by/SHA256SUMS` are included and verified. FaceSign never downloads or silently replaces these restricted face weights.

## Feature-vector migration

ArcFace uses a different feature space from SFace. Old SFace vectors cannot be converted and are never used for recognition. On first startup with this version, old vectors are moved to the `face_samples_archive` table and removed from the active face index. Student profiles, class and seat assignments, attendance history, camera configuration, user accounts, and application settings are preserved.

Students must be re-enrolled using the new model or imported from authorized original face photos to create new ArcFace vectors.

## Runtime

- HTTP: `0.0.0.0:8080`
- HTTPS: `0.0.0.0:8443`
- ONNX Runtime DLL: `onnxruntime.dll` beside `FaceSign.exe`
- Runtime support models: `models/`
- Face-recognition models: `models/by/` or `FACESIGN_BY_MODEL_DIR`

If startup fails, check `facesign-error.log` beside the executable. If the directory is not writable, the fallback log is under the user's temporary directory. Installer output is also recorded in `data/facesign-install.log`.

## Development and CI

Windows CI runs Go tests, vet, builds the Windows x64 executables, validates the current runtime model manifest, and creates package artifacts. End-to-end face recognition smoke tests require the authorized SCRFD and ArcFace weights to be supplied to the runner; they are not silently replaced by an older model.

## Ubuntu deployment

Ubuntu 22.04/24.04 AMD64 build and process scripts: see [README-UBUNTU.md](README-UBUNTU.md). The GitHub Actions workflow publishes the `facesign-ubuntu-amd64-full` artifact.
