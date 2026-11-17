# FaceSign By model files

The By runtime uses the following externally supplied ONNX files:

| File | Role | SHA-256 |
|---|---|---|
| `scrfd_10g.onnx` | SCRFD-10GF face detector with 5-point keypoints | `5838f7fe053675b1c7a08b633df49e7af5495cee0493c7dcf6697200b85b5b91` |
| `arcface_w600k_r50.onnx` | ArcFace / IResNet50 WebFace600K recognizer, 512-D | `4c06341c33c2ca1f86781dab0e829f88ad5b64be9fba56e56bc9ebdefc619e43` |

## Install

1. Obtain the exact model files from a source whose licence covers your intended use.
2. Put them in this directory, or set `FACESIGN_BY_MODEL_DIR` to another directory.
3. Keep the filenames above, and place a `SHA256SUMS` file alongside them.
4. Start FaceSign with the default `--face-model by` backend.

The resolver verifies both model files against `SHA256SUMS` and will not silently download third-party weights.

The reference InsightFace v0.7 `buffalo_l.zip` contains `det_10g.onnx` and `w600k_r50.onnx`; rename those two files to the names above if you have the appropriate licence. The original upstream model-zoo weights are marked for non-commercial research purposes only. Do not use them for operational school attendance or redistribute them unless you have obtained permission covering that use. The InsightFace code licence does not override the pretrained-weight terms.

Official sources:
- https://github.com/deepinsight/insightface/releases/tag/v0.7
- https://github.com/deepinsight/insightface/tree/master/model_zoo

## Optional landmark models

The resolver can also verify optional `2d106det.onnx` and `1k3d68.onnx` files if their hashes are added to `SHA256SUMS`. The current Go attendance pipeline uses SCRFD's five landmarks for ArcFace alignment; the 106-point and 3D-68 inference/display pipeline is not yet enabled.

Gender/age inference is intentionally excluded from attendance decisions.

## Migration

The By engine uses a separate 512-dimensional feature space. Existing SFace vectors are not convertible. On first startup with the By backend, FaceSign archives non-By vectors in the local `face_samples_legacy` table and removes them from the active face index. Student profiles, class/seat data, attendance, camera configuration, user accounts, and settings remain intact. Students must be enrolled again or imported from authorized source photos to create By vectors.
