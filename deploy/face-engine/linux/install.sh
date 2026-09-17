#!/bin/sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "Please run as root: sudo $0"
  exit 1
fi

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
SOURCE_BIN="$SCRIPT_DIR/facesign-face-engine"
SOURCE_ORT="$SCRIPT_DIR/libonnxruntime.so.1.26.0"
SOURCE_MODELS="$SCRIPT_DIR/models"
INSTALL_DIR=${FACESIGN_FACE_ENGINE_DIR:-/opt/facesign/face-engine}
DATA_DIR=${FACESIGN_FACE_ENGINE_DATA:-/var/lib/facesign/face-engine/data}
PORT=${FACESIGN_FACE_ENGINE_PORT:-18081}

for required in "$SOURCE_BIN" "$SOURCE_ORT" "$SOURCE_MODELS/face_detection_yunet_2023mar.onnx" "$SOURCE_MODELS/face_recognition_sface_2021dec.onnx"; do
  if [ ! -f "$required" ]; then
    echo "Release package is incomplete. Missing: $required" >&2
    exit 1
  fi
done

if ! id facesign >/dev/null 2>&1; then
  useradd --system --home /var/lib/facesign --shell /usr/sbin/nologin facesign
fi

systemctl stop facesign-face-engine.service >/dev/null 2>&1 || true
install -d -m 0755 "$INSTALL_DIR"
install -d -m 0750 -o facesign -g facesign "$DATA_DIR"
install -m 0755 "$SOURCE_BIN" "$INSTALL_DIR/facesign-face-engine"
install -m 0644 "$SOURCE_ORT" "$INSTALL_DIR/libonnxruntime.so.1.26.0"
rm -rf "$INSTALL_DIR/models"
cp -a "$SOURCE_MODELS" "$INSTALL_DIR/models"
chown -R root:root "$INSTALL_DIR"

cat > /etc/systemd/system/facesign-face-engine.service <<EOF
[Unit]
Description=FaceSign Native CPU Face Engine
After=network.target

[Service]
Type=simple
User=facesign
Group=facesign
ExecStart=$INSTALL_DIR/facesign-face-engine --listen 127.0.0.1:$PORT --assets $INSTALL_DIR --data $DATA_DIR/faces.db
Restart=always
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$DATA_DIR

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now facesign-face-engine.service

echo "Waiting for FaceSign native CPU face engine..."
i=0
while [ "$i" -lt 30 ]; do
  if command -v curl >/dev/null 2>&1; then
    if curl -fsS --max-time 3 "http://127.0.0.1:$PORT/health" >/dev/null 2>&1; then
      break
    fi
  elif systemctl is-active --quiet facesign-face-engine.service; then
    break
  fi
  i=$((i + 1))
  sleep 1
done

if [ "$i" -lt 30 ]; then
  echo "FaceSign native CPU face engine is ready."
  echo "Python: not required"
  echo "Docker: not required"
  echo "GPU/CUDA: not required"
  echo "URL: http://127.0.0.1:$PORT"
  echo "Data: $DATA_DIR/faces.db"
  exit 0
fi

systemctl --no-pager --full status facesign-face-engine.service || true
echo "Face engine did not become ready." >&2
exit 1
