#!/usr/bin/env bash
set -euo pipefail

VERSION="${COMPREFACE_VERSION:-1.2.0}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_DIR="${FACESIGN_COMPREFACE_DIR:-$SCRIPT_DIR/compreface}"
DOWNLOAD_URL="https://github.com/exadel-inc/CompreFace/releases/download/v${VERSION}/CompreFace_${VERSION}.zip"

fail() {
  echo "[FaceSign] $*" >&2
  exit 1
}

for cmd in docker curl unzip; do
  command -v "$cmd" >/dev/null 2>&1 || fail "缺少 $cmd。请先安装 Docker、curl 和 unzip 后重新运行。"
done

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
  trap 'rm -rf "$TMP_DIR"' EXIT
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

echo "[FaceSign] 等待服务启动（首次启动通常需要 30-90 秒）..."
READY=0
for _ in $(seq 1 45); do
  if curl -fsS --max-time 3 http://127.0.0.1:8000/ >/dev/null 2>&1; then
    READY=1
    break
  fi
  sleep 2
done

if [[ "$READY" -eq 1 ]]; then
  echo "[FaceSign] CompreFace 已启动：http://127.0.0.1:8000"
else
  echo "[FaceSign] 容器已经启动，但 8000 端口暂未就绪，请稍后打开 http://127.0.0.1:8000 查看。" >&2
fi

cat <<'EOF'

下一步：
1. 打开 http://127.0.0.1:8000，完成 CompreFace 初始化。
2. 创建 Application -> Face Recognition Service，并复制 API Key。
3. 回到 FaceSign -> 人脸识别设置，选择 CompreFace，填写 API Key 后保存并点击“检测服务”。
EOF
