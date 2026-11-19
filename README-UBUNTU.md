# FaceSign Ubuntu AMD64 deployment

This package targets Ubuntu 22.04 / 24.04 on x86-64. It contains the Linux FaceSign server, ONNX Runtime CPU shared libraries, the redistributable support models, and three process-management scripts.

## 1. Install system dependencies

```bash
sudo apt-get update
sudo apt-get install -y ca-certificates curl ffmpeg libstdc++6 libgomp1
```

FFmpeg is used for RTSP/network camera decoding. The Linux server currently supports browser cameras and network cameras; direct USB-camera enumeration through the FaceSign host is Windows-only.

## 2. Install the authorized SCRFD and ArcFace weights

The FaceSign code repository does not redistribute the InsightFace SCRFD/ArcFace pretrained weights. Obtain checkpoints whose licence explicitly permits your intended use, then place them in `models/by/`:

- `scrfd_10g.onnx`
- `arcface_w600k_r50.onnx`
- `SHA256SUMS`

Follow the exact architecture, provenance and checksum instructions in [models/by/README.md](models/by/README.md). Do not copy example checksum values; generate the manifest from the exact files you are licensed to use.

## 3. Start, inspect and stop

Run from the extracted package directory:

```bash
./scripts/ubuntu/start.sh
./scripts/ubuntu/status.sh
./scripts/ubuntu/stop.sh
```

The startup script runs FaceSign in the background, waits for the local health endpoint, and writes the PID and log files. It sends SIGTERM on stop and waits for a graceful shutdown.

Default locations:

- Database and TLS identity: `~/.local/share/facesign/`
- PID and logs: `~/.local/state/facesign/`
- Main log: `~/.local/state/facesign/facesign.log`

The scripts do not delete the database, TLS certificates or logs when stopping.

## 4. Network and TLS

Default listeners are HTTP `0.0.0.0:8080` and HTTPS `0.0.0.0:8443`. For remote browser access, use HTTPS and install/trust the FaceSign root CA on the client device. If you access the service by a LAN IP or DNS name, include that name in the certificate SANs when starting FaceSign:

```bash
FACESIGN_TLS_HOSTS="facesign.example.edu,192.168.1.20" ./scripts/ubuntu/start.sh
```

Open the HTTPS port in your firewall as required. The HTTP listener is retained for local health checks and does not redirect to HTTPS in this package mode.

## 5. Configuration overrides

The scripts support these environment variables:

- `FACESIGN_DATA_DIR`, `FACESIGN_STATE_DIR`
- `FACESIGN_PID_FILE`, `FACESIGN_LOG_FILE`
- `FACESIGN_BY_MODEL_DIR`
- `FACESIGN_HTTP_ADDR`, `FACESIGN_HTTPS_ADDR`
- `FACESIGN_HTTP_PORT`, `FACESIGN_HTTPS_PORT`
- `FACESIGN_TLS_DIR`, `FACESIGN_TLS_HOSTS`, `FACESIGN_DB_PATH`
- `FACESIGN_MATCH_THRESHOLD`, `FACESIGN_DETECTION_THRESHOLD`
- `FACESIGN_HEALTH_URL`

If you override data/state paths when starting, pass the same overrides to the status and stop scripts.

## 6. Model and security notes

The package includes the default Silent-Face MiniFASNetV2 + MiniFASNetV1SE anti-spoof models, the YOLOX-Nano person detector, and the legacy MN3 / MiniFASNetV2-SE fallback models with their source checksums and licence files. SCRFD/ArcFace weights are supplied separately by the operator and are checked against the local SHA256 manifest before startup.

The liveness model remains enabled by default. Avoid setting `FACESIGN_LIVENESS_BACKEND` to a legacy backend unless the matching model file is present and you intend to run that comparison.
