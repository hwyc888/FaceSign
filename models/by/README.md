# FaceSign By face models

The By branch has one supported face pipeline:

- `scrfd_10g.onnx`: SCRFD-10GF face detector (expected graph: one NCHW float input, nine outputs for strides 8/16/32 with two anchors and five landmarks).
- `arcface_w600k_r50.onnx`: ArcFace / IResNet50 recognizer (112x112 BGR input, 512-dimensional float output).

Put both authorized model files in this directory, or set `FACESIGN_BY_MODEL_DIR`.

## Create the local checksum manifest

The application requires a SHA-256 manifest to detect accidental or incomplete model-file changes. The manifest is generated locally from the exact files you obtained; this is an integrity check, not proof of publisher authenticity.

In PowerShell, run:

```powershell
Get-FileHash .\scrfd_10g.onnx -Algorithm SHA256
Get-FileHash .\arcface_w600k_r50.onnx -Algorithm SHA256
```

Create `SHA256SUMS` in this directory with these two lines, using the actual hashes printed on your computer:

```text
<64-character-SCRFD-SHA256>  scrfd_10g.onnx
<64-character-ArcFace-SHA256>  arcface_w600k_r50.onnx
```

Do not copy example hashes from documentation. FaceSign checks both files against this manifest before loading them.

## Model source and licensing

The upstream InsightFace model zoo's pretrained weights are labelled for non-commercial research use. The software repository's code licence does not automatically grant broader rights to pretrained weights. Do not use those weights for operational school attendance unless you have written permission or another exact checkpoint licence that covers the intended use and redistribution.

Official references:
- https://github.com/deepinsight/insightface/releases/tag/v0.7
- https://github.com/deepinsight/insightface/tree/master/model_zoo

The names above are the FaceSign local filenames. For the buffalo_l package, the corresponding source files are commonly named `det_10g.onnx` and `w600k_r50.onnx`; rename only after verifying the checkpoint architecture and licence.

## Enrollment and migration

ArcFace embeddings are a separate feature space from SFace. FaceSign does not convert old vectors. On first startup with the new backend, old vectors are archived in `face_samples_archive` and removed from the active index. Student profiles, class/seat data, attendance, camera configuration, user accounts, and settings remain. Students must be re-enrolled or imported from authorized original photos.

106-point and 3D-68 landmark inference are not part of the current attendance pipeline. Gender/age estimation is intentionally excluded.
