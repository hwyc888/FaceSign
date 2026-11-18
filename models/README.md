# FaceSign runtime models

FaceSign By uses SCRFD-10GF + ArcFace ResNet50 for identity recognition. Place independently authorized weights under `models/by/` following [the installation guide](by/README.md).

## MiniFASNetV2-SE trial (default)

- Filename: `minifasnet-v2se.onnx` (official [facenox/face-antispoof-onnx v1.0.0](https://github.com/facenox/face-antispoof-onnx/releases/download/v1.0.0/best_model.onnx)).
- Expected SHA-256: `af2381b88f38769222ed93379e12444e2a50814575de1c46170de570c55a42b6`; FaceSign verifies the hash before loading a downloaded model.
- Preprocessing: 1.5x square face crop (reflection padding), 128x128 RGB CHW [0,1]. ONNX class 0 = real; class 1 = spoof.
- To roll back: set `FACESIGN_LIVENESS_BACKEND=mn3` **before launching FaceSign**, then restart. The old `anti-spoof-mn3.onnx` remains included in the full bundle.
- The active model name appears in the check-in result panel and startup logs.
- Both models retain the same three-frame PAD rules and photo/screen rejection. Identity matching alone never bypasses PAD.

**Noncommercial trial:** The facenox repository is Apache-2.0 (license copied to `MINIFAS_LICENSE.txt`), but its checkpoint is trained on CelebA-Spoof, which limits dataset usage to noncommercial research. Confirm weight/training-data permissions before commercial use or redistribution beyond permitted evaluation.

The other support model is `yolox_nano.onnx`, used for person tracking. The legacy anti-spoof model is retained solely as a rollback option.
