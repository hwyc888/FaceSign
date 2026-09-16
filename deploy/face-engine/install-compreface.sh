#!/usr/bin/env bash
set -euo pipefail

VERSION="${COMPREFACE_VERSION:-1.2.0}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_DIR="${FACESIGN_COMPREFACE_DIR:-$SCRIPT_DIR/compreface}"
SERVICE_URL="${FACESIGN_COMPREFACE_URL:-http://127.0.0.1:8000}"
FACESIGN_BIN="${FACESIGN_BIN:-$SCRIPT_DIR/../facesign}"
FACESIGN_ENV_FILE="${FACESIGN_ENV_FILE:-}"
DOWNLOAD_URL="https://github.com/exadel-inc/CompreFace/releases/download/v${VERSION}/CompreFace_${VERSION}.zip"
CREDENTIALS_FILE="$INSTALL_DIR/facesign-admin.env"
GENERATED_ENV_FILE="$INSTALL_DIR/facesign-face.env"
TMP_DIR=""

fail() {
  echo "[FaceSign] $*" >&2
  exit 1
}

cleanup() {
  if [[ -n "$TMP_DIR" && -d "$TMP_DIR" ]]; then
    rm -rf "$TMP_DIR"
  fi
}
trap cleanup EXIT

update_env_value() {
  local file="$1" key="$2" value="$3" temp
  mkdir -p "$(dirname "$file")"
  touch "$file"
  temp="$(mktemp "${file}.XXXXXX")"
  awk -v key="$key" -v value="$value" '
    BEGIN { found = 0 }
    index($0, key "=") == 1 { print key "=" value; found = 1; next }
    { print }
    END { if (!found) print key "=" value }
  ' "$file" > "$temp"
  mv "$temp" "$file"
}

for cmd in docker curl unzip awk od; do
  command -v "$cmd" >/dev/null 2>&1 || fail "缺少 $cmd。请先安装 Docker、curl 和 unzip 后重新运行。"
done
[[ -x "$FACESIGN_BIN" ]] || fail "未找到可执行的 FaceSign：$FACESIGN_BIN。请从完整 Linux 发布包运行 install.sh。"

docker info >/dev/null 2>&1 || fail "Docker 当前不可用。请先启动 Docker 服务，并确认当前用户有权限运行 docker。"

if docker compose version >/dev/null 2>&1; then
  COMPOSE=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE=(docker-compose)
else
  fail "未检测到 Docker Compose。请安装 Docker Compose 后重试。"
fi

if [[ -r /proc/cpuinfo ]] && ! grep -qm1 -w avx /proc/cpuinfo; then
  fail "当前 CPU 未检测到 AVX 指令集，官方 CompreFace 默认镜像无法正常运行。"
fi

if [[ ! -f "$INSTALL_DIR/docker-compose.yml" ]]; then
  echo "[FaceSign] 下载官方 CompreFace ${VERSION}..."
  TMP_DIR="$(mktemp -d)"
  curl -fL "$DOWNLOAD_URL" -o "$TMP_DIR/compreface.zip"
  mkdir -p "$TMP_DIR/extract"
  unzip -q "$TMP_DIR/compreface.zip" -d "$TMP_DIR/extract"
  COMPOSE_FILE="$(find "$TMP_DIR/extract" -name docker-compose.yml -type f -print -quit)"
  [[ -n "$COMPOSE_FILE" ]] || fail "下载包中没有找到 docker-compose.yml"
  mkdir -p "$INSTALL_DIR"
  cp -a "$(dirname "$COMPOSE_FILE")/." "$INSTALL_DIR/"
fi

echo "[FaceSign] 启动 CompreFace..."
(
  cd "$INSTALL_DIR"
  "${COMPOSE[@]}" up -d
)

echo "[FaceSign] 等待 CompreFace 管理接口就绪（首次启动可能需要数分钟）..."
READY=0
for _ in $(seq 1 180); do
  if curl -fsS --max-time 5 "$SERVICE_URL/admin/config" >/dev/null 2>&1; then
    READY=1
    break
  fi
  sleep 2
done
[[ "$READY" -eq 1 ]] || fail "CompreFace 容器已启动，但管理接口未能在 6 分钟内就绪。请运行 docker compose logs 检查。"

if [[ -f "$CREDENTIALS_FILE" ]]; then
  # This file is created by this script with restrictive permissions and simple key/value content.
  # shellcheck disable=SC1090
  source "$CREDENTIALS_FILE"
else
  COMPREFACE_ADMIN_EMAIL="${COMPREFACE_ADMIN_EMAIL:-facesign@local.invalid}"
  COMPREFACE_ADMIN_PASSWORD="${COMPREFACE_ADMIN_PASSWORD:-$(od -An -N24 -tx1 /dev/urandom | tr -d ' \n')}"
  umask 077
  mkdir -p "$INSTALL_DIR"
  printf 'COMPREFACE_ADMIN_EMAIL=%s\nCOMPREFACE_ADMIN_PASSWORD=%s\n' \
    "$COMPREFACE_ADMIN_EMAIL" "$COMPREFACE_ADMIN_PASSWORD" > "$CREDENTIALS_FILE"
fi
chmod 0600 "$CREDENTIALS_FILE"

echo "[FaceSign] 自动创建或复用 CompreFace 应用和人脸识别服务..."
if ! PROVISION_OUTPUT="$(
  COMPREFACE_ADMIN_EMAIL="$COMPREFACE_ADMIN_EMAIL" \
  COMPREFACE_ADMIN_PASSWORD="$COMPREFACE_ADMIN_PASSWORD" \
  "$FACESIGN_BIN" provision-compreface --url "$SERVICE_URL" --email "$COMPREFACE_ADMIN_EMAIL" 2>&1
)"; then
  echo "$PROVISION_OUTPUT" >&2
  fail "CompreFace 自动初始化失败。若该实例已由其他账号初始化，请删除 $CREDENTIALS_FILE 后通过 COMPREFACE_ADMIN_EMAIL/COMPREFACE_ADMIN_PASSWORD 传入已有管理员凭据重试。"
fi
API_KEY="$(printf '%s\n' "$PROVISION_OUTPUT" | tail -n 1 | tr -d '\r')"
[[ -n "$API_KEY" ]] || fail "CompreFace 自动初始化未返回 API Key"

umask 077
cat > "$GENERATED_ENV_FILE" <<ENV
FACESIGN_FACE_PROVIDER=compreface
FACESIGN_COMPREFACE_URL=$SERVICE_URL
FACESIGN_COMPREFACE_API_KEY=$API_KEY
ENV
chmod 0600 "$GENERATED_ENV_FILE"

if [[ -n "$FACESIGN_ENV_FILE" ]]; then
  update_env_value "$FACESIGN_ENV_FILE" FACESIGN_FACE_PROVIDER compreface
  update_env_value "$FACESIGN_ENV_FILE" FACESIGN_COMPREFACE_URL "$SERVICE_URL"
  update_env_value "$FACESIGN_ENV_FILE" FACESIGN_COMPREFACE_API_KEY "$API_KEY"
fi

echo "[FaceSign] CompreFace 已就绪，识别服务和 API Key 已自动配置。"
echo "[FaceSign] 管理员凭据保存在：$CREDENTIALS_FILE"
if [[ -n "$FACESIGN_ENV_FILE" ]]; then
  echo "[FaceSign] FaceSign 配置已更新：$FACESIGN_ENV_FILE"
else
  echo "[FaceSign] 可导入的配置已生成：$GENERATED_ENV_FILE"
fi
