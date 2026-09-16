#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_BIN="$SCRIPT_DIR/facesign"
SKIP_FACE_ENGINE=0

for arg in "$@"; do
  case "$arg" in
    --skip-face-engine) SKIP_FACE_ENGINE=1 ;;
    *) SOURCE_BIN="$arg" ;;
  esac
done

if [[ "$(id -u)" -ne 0 ]]; then
  echo "请以 root 运行：sudo $0 [--skip-face-engine]" >&2
  exit 1
fi
if [[ ! -f "$SOURCE_BIN" ]]; then
  echo "未找到 FaceSign 可执行文件：$SOURCE_BIN" >&2
  exit 1
fi

if ! id facesign >/dev/null 2>&1; then
  useradd --system --home /var/lib/facesign --shell /usr/sbin/nologin facesign
fi

install -d -m 0755 /opt/facesign
install -d -m 0755 /opt/facesign/face-engine
install -d -m 0750 -o facesign -g facesign /var/lib/facesign
install -d -m 0750 -o root -g facesign /etc/facesign
install -m 0755 "$SOURCE_BIN" /opt/facesign/facesign

ENV_FILE=/etc/facesign/facesign.env
if [[ ! -f "$ENV_FILE" ]]; then
  KIOSK_KEY="$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')"
  cat > "$ENV_FILE" <<ENV
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
fi
chown root:facesign "$ENV_FILE"
chmod 0640 "$ENV_FILE"

if [[ "$SKIP_FACE_ENGINE" -eq 0 ]]; then
  ENGINE_SCRIPT="$SCRIPT_DIR/face-engine/install-compreface.sh"
  [[ -f "$ENGINE_SCRIPT" ]] || {
    echo "发布包不完整：缺少 $ENGINE_SCRIPT" >&2
    exit 1
  }
  install -m 0755 "$ENGINE_SCRIPT" /opt/facesign/face-engine/install-compreface.sh
  FACESIGN_BIN=/opt/facesign/facesign \
  FACESIGN_ENV_FILE="$ENV_FILE" \
  FACESIGN_COMPREFACE_DIR=/opt/facesign/face-engine/compreface \
    /opt/facesign/face-engine/install-compreface.sh
  chown root:facesign "$ENV_FILE"
  chmod 0640 "$ENV_FILE"
fi

install -m 0644 "$SCRIPT_DIR/facesign.service" /etc/systemd/system/facesign.service
systemctl daemon-reload
systemctl enable --now facesign.service
systemctl restart facesign.service

echo "[FaceSign] 正在验证服务..."
READY=0
for _ in $(seq 1 30); do
  if curl -fsS --max-time 3 http://127.0.0.1:8080/api/health >/dev/null 2>&1; then
    READY=1
    break
  fi
  sleep 1
done
if [[ "$READY" -ne 1 ]]; then
  systemctl status facesign.service --no-pager >&2 || true
  exit 1
fi

if [[ "$SKIP_FACE_ENGINE" -eq 0 ]]; then
  STATUS="$(curl -fsS --max-time 10 http://127.0.0.1:8080/api/kiosk/status)"
  if [[ "$STATUS" != *'"face_ready":true'* ]]; then
    echo "[FaceSign] FaceSign 已启动，但人脸识别服务自检失败：$STATUS" >&2
    exit 1
  fi
fi

if [[ "$SKIP_FACE_ENGINE" -eq 0 ]]; then
  echo "[FaceSign] 安装完成，FaceSign 和人脸识别服务均已通过检查。"
else
  echo "[FaceSign] FaceSign 安装完成；已按要求跳过人脸识别服务。"
fi
echo "[FaceSign] 配置文件：$ENV_FILE"
echo "[FaceSign] 服务状态：systemctl status facesign"
echo "[FaceSign] 管理端：http://SERVER_IP:8080/"
