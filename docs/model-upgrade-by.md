# FaceSign model upgrade track (By)

This branch is an isolated model-upgrade experiment. It must not change the
production model selection on `main` until accuracy, latency, memory,
licensing, and migration gates have passed.

## Current production stack

| Role | Current implementation |
|---|---|
| Face detector | YuNet ONNX, 5 facial landmarks |
| Face embedding | OpenCV SFace ONNX |
| Passive presentation-attack detection | anti-spoof-mn3 ONNX (MobileNetV3 family) |
| Person detection/tracking input | YOLOX-Nano ONNX; Go IoU/center-distance track association |
| Runtime | ONNX Runtime CPU, Go application |

## Candidate evaluation

| Role | Candidate | Status |
|---|---|---|
| Face detector | SCRFD-10GF | Candidate; adapter and output decoder not yet integrated |
| Face embedding | ResNet50 trained on WebFace600K | Candidate; embedding contract and licence must be confirmed |
| 2D landmarks | 106-point model | Optional; not required for initial recognition migration |
| 3D landmarks | 68-point model | Deferred; separate pose/geometry capability |
| Age/gender | Attribute model | Out of scope for attendance; do not collect unless a separate lawful purpose exists |
| Person tracking | Existing YOLOX-Nano + Go tracker | Retain |
| PAD/liveness | Existing anti-spoof-mn3 + multi-frame vote | Retain as baseline; evaluate separately |

## Licensing gate

Do not download, commit, package, or distribute third-party pretrained weights
until the exact checkpoint's licence permits the intended school/competition
deployment and redistribution. InsightFace code licensing does not automatically
grant commercial rights to its pretrained model weights. Obtain a written
licence or use weights with terms compatible with the deployment.

Experimental model files must be supplied locally by an authorized party and
must not be committed to this repository. Record the upstream URL, exact
checkpoint/version, licence evidence, SHA-256, input tensor contract, output
tensor contract, preprocessing, and postprocessing in the test report.

## Safe integration rules

1. Keep YuNet + SFace as the production default and fallback.
2. Keep candidate models in a separate model directory and namespace.
3. Verify every candidate file with SHA-256 before loading.
4. Never reuse SFace embeddings or thresholds with a different recognizer.
5. Build a new feature-space version and re-extract authorized enrollment
   samples before comparing the candidate recognizer.
6. Do not silently fall back after an inference error; report the active model
   and the failure clearly.
7. Keep passive PAD enabled by default. A high identity score is not proof of
   liveness.
8. Do not use age/gender estimates for attendance decisions.

## Benchmark protocol

Use the same consented and access-controlled test set for both pipelines.
Split by identity, not by image, to prevent identity leakage between enrollment
and test sets. Report:

- face detection recall by face-size, pose, illumination, and crowd-density bins;
- false match rate (FMR), false non-match rate (FNMR), ROC/DET and TAR at a
  stated FAR;
- PAD APCER, BPCER and ACER on separately documented bona-fide and attack sets;
- end-to-end p50/p95 latency, CPU utilization, peak memory and throughput;
- results on the actual target Windows CPU and camera resolutions.

Do not publish a single generic "accuracy" percentage without the test protocol.

## Implementation status in this branch

The first change adds a SHA-256-verified manifest-validation primitive for
locally supplied experimental model files and regression tests. It does not
yet implement SCRFD/ResNet50 inference or change runtime model selection.
The next implementation step is to confirm authorized model artifacts and
their ONNX tensor contracts, then implement and test the detector/recognizer
adapters behind an explicit experimental configuration.
