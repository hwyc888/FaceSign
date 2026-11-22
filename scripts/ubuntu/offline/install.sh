#!/usr/bin/env bash
set -Eeuo pipefail

BUNDLE_DIR="$(cd -- "$(dirname -- "$0")" && pwd)"
MODE=full
if [[ "$#" -gt 0 ]]; then MODE="$1"; fi
APP_DIR=/opt/facesign
if [[ -n "${FACESIGN_INSTALL_DIR:-}" ]]; then APP_DIR="$FACESIGN_INSTALL_DIR"; fi

echo "=================================================="
echo " FaceSign Ubuntu 24.04 AMD64 Offline Installer"
echo "=================================================="

if [[ "$EUID" -ne 0 ]]; then
  echo "ERROR: run this installer as root." >&2
  exit 1
fi

if [[ ! -r /etc/os-release ]]; then
  echo "ERROR: cannot identify the target operating system." >&2
  exit 1
fi
. /etc/os-release
if [[ "${ID:-}" != "ubuntu" || "${VERSION_ID:-}" != "24.04" ]]; then
  echo "ERROR: this offline bundle targets Ubuntu 24.04 only." >&2
  exit 1
fi
if [[ "$(dpkg --print-architecture)" != "amd64" ]]; then
  echo "ERROR: this offline bundle requires AMD64." >&2
  exit 1
fi

DEB_FILES=("$BUNDLE_DIR"/debs/*.deb)
if [[ "${#DEB_FILES[@]}" -eq 0 || ! -f "${DEB_FILES[0]}" ]]; then
  echo "ERROR: no offline DEB packages found in $BUNDLE_DIR/debs" >&2
  exit 1
fi

if [[ -f "$BUNDLE_DIR/debs/SHA256SUMS" ]]; then
  echo "[1/6] Verifying offline DEB checksums..."
  (cd "$BUNDLE_DIR/debs" && sha256sum -c SHA256SUMS)
else
  echo "WARNING: debs/SHA256SUMS is absent; package checksums are not verified."
fi

echo "[2/6] Installing system packages without network access..."
DEBIAN_FRONTEND=noninteractive apt-get install --no-download -y "${DEB_FILES[@]}"

RUNTIME_DIR="$BUNDLE_DIR/runtime/facesign"
if [[ ! -x "$RUNTIME_DIR/FaceSign" ]]; then
  echo "ERROR: FaceSign executable is missing from runtime/facesign." >&2
  exit 1
fi

echo "[3/6] Installing FaceSign runtime into $APP_DIR..."
mkdir -p "$APP_DIR"
cp -a "$RUNTIME_DIR/." "$APP_DIR/"
chmod 755 "$APP_DIR/FaceSign"
chmod 755 "$APP_DIR"/scripts/ubuntu/*.sh

echo "[4/6] Checking FFmpeg and ONNX Runtime..."
command -v curl >/dev/null 2>&1 || {
  echo "ERROR: curl is missing after offline package installation." >&2
  exit 1
}
command -v ffmpeg >/dev/null 2>&1 || {
  echo "ERROR: ffmpeg is missing after offline package installation." >&2
  exit 1
}
command -v ffprobe >/dev/null 2>&1 || {
  echo "ERROR: ffprobe is missing after offline package installation." >&2
  exit 1
}

for file in \
  "$APP_DIR/libonnxruntime.so" \
  "$APP_DIR/libonnxruntime.so.1.26.0" \
  "$APP_DIR/libonnxruntime_providers_shared.so"
do
  if [[ ! -f "$file" ]]; then
    echo "ERROR: missing ONNX Runtime library: $file" >&2
    exit 1
  fi
  output="$(LD_LIBRARY_PATH="$APP_DIR:${LD_LIBRARY_PATH:-}" ldd "$file" 2>&1 || true)"
  if grep -q "not found" <<< "$output"; then
    echo "ERROR: unresolved ONNX Runtime dependency: $file" >&2
    echo "$output" >&2
    exit 1
  fi
done

output="$(LD_LIBRARY_PATH="$APP_DIR:${LD_LIBRARY_PATH:-}" ldd "$APP_DIR/FaceSign" 2>&1 || true)"
if grep -q "not found" <<< "$output"; then
  echo "ERROR: FaceSign has unresolved shared-library dependencies." >&2
  echo "$output" >&2
  exit 1
fi

echo "[5/6] Checking SCRFD and ArcFace model files..."
MODEL_DIR="$APP_DIR/models/by"
mkdir -p "$MODEL_DIR"

# Normalize the common buffalo_l filenames without deleting the originals.
if [[ ! -f "$MODEL_DIR/scrfd_10g.onnx" && -f "$MODEL_DIR/det_10g.onnx" ]]; then
  cp -n "$MODEL_DIR/det_10g.onnx" "$MODEL_DIR/scrfd_10g.onnx"
fi
if [[ ! -f "$MODEL_DIR/arcface_w600k_r50.onnx" && -f "$MODEL_DIR/w600k_r50.onnx" ]]; then
  cp -n "$MODEL_DIR/w600k_r50.onnx" "$MODEL_DIR/arcface_w600k_r50.onnx"
fi

if [[ -f "$MODEL_DIR/scrfd_10g.onnx" && -f "$MODEL_DIR/arcface_w600k_r50.onnx" ]]; then
  if [[ ! -s "$MODEL_DIR/SHA256SUMS" ]]; then
    echo "Generating SHA256SUMS from the exact installed model files..."
    (cd "$MODEL_DIR" && sha256sum scrfd_10g.onnx arcface_w600k_r50.onnx > SHA256SUMS)
  fi
  if ! grep -Eq '  scrfd_10g\.onnx$' "$MODEL_DIR/SHA256SUMS" ||
     ! grep -Eq '  arcface_w600k_r50\.onnx$' "$MODEL_DIR/SHA256SUMS"; then
    echo "ERROR: SHA256SUMS does not reference the required FaceSign model filenames." >&2
    echo "Move the old manifest aside, then regenerate it with sha256sum." >&2
    exit 1
  fi
  (cd "$MODEL_DIR" && sha256sum -c SHA256SUMS)
else
  echo "Runtime installed, but SCRFD/ArcFace identity weights are not both present."
  if [[ "$MODE" == "runtime-only" ]]; then
    echo "Runtime-only installation completed. Add authorized models before starting FaceSign."
    exit 0
  fi
  echo "Add scrfd_10g.onnx, arcface_w600k_r50.onnx and SHA256SUMS to $MODEL_DIR." >&2
  echo "No FaceSign process was started." >&2
  exit 4
fi

echo "[6/6] Starting FaceSign..."
if [[ "${FACESIGN_AUTO_START:-1}" == "1" ]]; then
  "$APP_DIR/scripts/ubuntu/start.sh"
  "$APP_DIR/scripts/ubuntu/status.sh"
else
  echo "Automatic start disabled by FACESIGN_AUTO_START=0."
fi

echo "=================================================="
echo " FaceSign offline installation completed"
echo "=================================================="
echo "FFmpeg: $(command -v ffmpeg)"
echo "FaceSign: $APP_DIR/FaceSign"
echo "HTTPS: https://localhost:8443/"
