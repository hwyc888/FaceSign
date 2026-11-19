# FaceSign

FaceSign is a compact school face-attendance application written in Go for Windows.

## Face-recognition stack

This branch has one supported face-recognition pipeline:

- **SCRFD-10GF**: face detection and five facial landmarks.
- **ArcFace ResNet50**: 512-dimensional identity embeddings.
- **MiniFASNetV2-SE (trial)**: default RGB presentation-attack screening; MN3 remains selectable with `FACESIGN_LIVENESS_BACKEND=mn3` for A/B comparison (see [model terms](models/README.md)).
- **YOLOX-Nano**: person detection used by the person-tracking workflow.
- **ONNX Runtime CPU**: inference without a GPU requirement.

YuNet and SFace have been removed from this branch. There is no legacy face-model backend or runtime fallback.

Install the separately authorized SCRFD and ArcFace ONNX files under `models/by/`. See [models/by/README.md](models/by/README.md) for filenames and checksum-manifest setup. The face weights are not included in this repository or CI artifacts.

## Features

- Student and class management, including seat assignments.
- Browser camera enrollment and photo ZIP import.
- Face recognition and daily check-in.
- One check-in record per student per day.
- Attendance history and class seat board.
- Embedded responsive web UI.
- Local SQLite database.
- Portable Windows operation; optional startup registration through FaceSignManager.
- USB and RTSP camera support through the bundled FFmpeg runtime.

## Windows packages

GitHub Actions publishes:

- `facesign-windows-amd64-full`: application, native runtime, FFmpeg, anti-spoof model, person model, and By model installation instructions. Add authorized SCRFD/ArcFace files before first launch.
- `facesign-windows-amd64-lite`: lightweight application upgrade package without model files.
- `facesign-windows-amd64-full-no-ffmpeg`: full package without the FFmpeg runtime.
- `facesign-windows-amd64-no-models-no-ffmpeg`: minimal application package.
- `facesign-camera-agent-windows-amd64`: remote camera agent.

The full package is not self-contained for face inference until the two authorized By face weights and local SHA-256 manifest are installed. FaceSign never downloads or redistributes those weights automatically.

## Feature-vector migration

ArcFace uses a different feature space from SFace. Old SFace vectors cannot be converted and are never used for recognition. On first startup with this version, old vectors are moved to the `face_samples_archive` table and removed from the active face index. Student profiles, class and seat assignments, attendance history, camera configuration, user accounts, and application settings are preserved.

Students must be re-enrolled using the new model or imported from authorized original face photos to create new ArcFace vectors.

## Runtime

- HTTP: `0.0.0.0:8080`
- HTTPS: `0.0.0.0:8443`
- ONNX Runtime DLL: `onnxruntime.dll` beside `FaceSign.exe`
- Model directory: `models/by/` or `FACESIGN_BY_MODEL_DIR`

If startup fails, check `facesign-error.log` beside the executable. If the directory is not writable, the fallback log is under the user's temporary directory.

## Development and CI

Windows CI runs Go tests, vet, builds the Windows x64 executables, and creates package artifacts. End-to-end camera/model smoke tests require the authorized SCRFD and ArcFace weights to be supplied to the runner; they are not silently replaced by an older model.


## Ubuntu deployment

Ubuntu 22.04/24.04 AMD64 build and process scripts: see [README-UBUNTU.md](README-UBUNTU.md). The GitHub Actions workflow publishes the `facesign-ubuntu-amd64-full` artifact.
