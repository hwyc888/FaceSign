#!/usr/bin/env bash
set -euo pipefail

PORT="${FACESIGN_FACE_ENGINE_PORT:-18081}"
INSTALL_DIR="${FACESIGN_FACE_ENGINE_DIR:-/opt/facesign-face-engine}"
DATA_DIR="${FACESIGN_FACE_ENGINE_DATA:-/var/lib/facesign-face-engine}"
SERVICE_NAME="facesign-face-engine"
SERVICE_USER="facesign-face"
YUNET_URL="https://github.com/opencv/opencv_zoo/raw/main/models/face_detection_yunet/face_detection_yunet_2023mar.onnx"
SFACE_URL="https://github.com/opencv/opencv_zoo/raw/main/models/face_recognition_sface/face_recognition_sface_2021dec.onnx"

fail() { echo "[FaceSign] $*" >&2; exit 1; }

if [[ "${EUID}" -ne 0 ]]; then
  command -v sudo >/dev/null 2>&1 || fail "Please run as root or install sudo."
  exec sudo -E bash "$0" "$@"
fi

if ! command -v python3 >/dev/null 2>&1 || ! python3 -m venv --help >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y python3 python3-venv curl ca-certificates
  else
    fail "Python 3 with venv is required. Install python3, python3-venv, curl and retry."
  fi
fi
command -v curl >/dev/null 2>&1 || fail "curl is required."
command -v systemctl >/dev/null 2>&1 || fail "systemd is required for service installation."

if ! id "$SERVICE_USER" >/dev/null 2>&1; then
  useradd --system --home "$DATA_DIR" --shell /usr/sbin/nologin "$SERVICE_USER"
fi

mkdir -p "$INSTALL_DIR/models" "$DATA_DIR"
python3 -m venv "$INSTALL_DIR/venv"
"$INSTALL_DIR/venv/bin/python" -m pip install --disable-pip-version-check --upgrade pip
"$INSTALL_DIR/venv/bin/python" -m pip install --disable-pip-version-check \
  "numpy==1.26.4" \
  "opencv-python-headless==4.10.0.84" \
  "fastapi==0.115.6" \
  "uvicorn==0.34.0" \
  "python-multipart==0.0.20"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
[[ -f "$SCRIPT_DIR/face_engine.py" ]] || fail "face_engine.py is missing from the release package."
install -m 0644 "$SCRIPT_DIR/face_engine.py" "$INSTALL_DIR/face_engine.py"

fetch_model() {
  local url="$1" target="$2" minimum="$3"
  if [[ -f "$target" ]] && [[ "$(stat -c%s "$target")" -ge "$minimum" ]]; then return; fi
  echo "[FaceSign] Downloading $(basename "$target")..."
  curl -fL --retry 3 "$url" -o "$target"
  [[ "$(stat -c%s "$target")" -ge "$minimum" ]] || fail "Downloaded model is unexpectedly small: $target"
}
fetch_model "$YUNET_URL" "$INSTALL_DIR/models/face_detection_yunet_2023mar.onnx" 100000
fetch_model "$SFACE_URL" "$INSTALL_DIR/models/face_recognition_sface_2021dec.onnx" 10000000

chown -R root:root "$INSTALL_DIR"
chown -R "$SERVICE_USER:$SERVICE_USER" "$DATA_DIR"

cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<EOF
[Unit]
Description=FaceSign Local CPU Face Recognition Engine
After=network.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_USER}
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/venv/bin/python ${INSTALL_DIR}/face_engine.py --host 127.0.0.1 --port ${PORT} --models ${INSTALL_DIR}/models --data ${DATA_DIR}/faces.db
Restart=always
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=${DATA_DIR}

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now "$SERVICE_NAME"

echo "[FaceSign] Waiting for local CPU face engine..."
ready=0
for _ in $(seq 1 60); do
  if curl -fsS --max-time 3 "http://127.0.0.1:${PORT}/health" >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 2
done
[[ "$ready" -eq 1 ]] || { systemctl status "$SERVICE_NAME" --no-pager || true; fail "Local CPU face engine did not become ready."; }

cat <<EOF

[FaceSign] Local CPU face engine is ready.
  Device: CPU only (GPU/CUDA not required)
  Docker: not required
  URL: http://127.0.0.1:${PORT}

In FaceSign -> Face Recognition Settings:
  Engine: Local CPU Engine
  Service URL: http://127.0.0.1:${PORT}
  API Key: leave blank
  Recommended similarity threshold: 0.72
Then save and click Check Service.
EOF
