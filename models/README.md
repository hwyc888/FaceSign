# FaceSign pinned face models

These two ONNX files are intentionally stored once and are not copied into normal Windows program build artifacts.

Pinned binary commit:

`5f4dbed1d5e78b95b9af95f492f363d654bb01d9`

Files:

- `face_detection_yunet_2023mar.onnx`
- `face_recognition_sface_2021dec.onnx`

Integrity values are listed in `SHA256SUMS`.

The Windows installer reuses already installed valid models. On a fresh machine it downloads the models from the pinned commit once and verifies SHA-256 before starting FaceSign.
