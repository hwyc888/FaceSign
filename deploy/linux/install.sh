#!/bin/sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "Please run as root: sudo $0 /path/to/facesign-linux-amd64"
  exit 1
fi

SOURCE_BIN=${1:-./facesign-linux-amd64}
if [ ! -f "$SOURCE_BIN" ]; then
  echo "Binary not found: $SOURCE_BIN"
  exit 1
fi

if ! id facesign >/dev/null 2>&1; then
  useradd --system --home /var/lib/facesign --shell /usr/sbin/nologin facesign
fi

install -d -m 0755 /opt/facesign
install -d -m 0750 -o facesign -g facesign /var/lib/facesign
install -d -m 0750 /etc/facesign
install -m 0755 "$SOURCE_BIN" /opt/facesign/facesign

if [ ! -f /etc/facesign/facesign.env ]; then
  KIOSK_KEY=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')
  cat > /etc/facesign/facesign.env <<ENV
FACESIGN_ADDR=:8080
FACESIGN_DB=/var/lib/facesign/facesign.db
FACESIGN_TIMEZONE=Asia/Shanghai
FACESIGN_SESSION_HOURS=12
FACESIGN_KIOSK_ACCESS_KEY=$KIOSK_KEY
FACESIGN_FACE_PROVIDER=disabled
FACESIGN_COMPREFACE_URL=http://127.0.0.1:8000
FACESIGN_COMPREFACE_API_KEY=
FACESIGN_FACE_SIMILARITY=0.78
FACESIGN_FACE_DETECTION_THRESHOLD=0.80
FACESIGN_TLS_CERT=
FACESIGN_TLS_KEY=
FACESIGN_COOKIE_SECURE=false
ENV
  chmod 0600 /etc/facesign/facesign.env
fi

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
install -m 0644 "$SCRIPT_DIR/facesign.service" /etc/systemd/system/facesign.service
systemctl daemon-reload
systemctl enable --now facesign.service

echo "FaceSign installed."
echo "Configuration: /etc/facesign/facesign.env"
echo "Status: systemctl status facesign"
echo "Open: http://SERVER_IP:8080/"
