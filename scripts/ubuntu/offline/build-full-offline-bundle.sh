#!/usr/bin/env bash
set -Eeuo pipefail

APP_DIR=/opt/facesign
DEB_DIR=
LICENSE_FILE=
OUTPUT="$PWD/FaceSign-Ubuntu-24.04-AMD64-Full-Offline.tar.gz"

if [[ "$#" -ge 1 ]]; then APP_DIR="$1"; fi
if [[ "$#" -ge 2 ]]; then DEB_DIR="$2"; fi
if [[ "$#" -ge 3 ]]; then LICENSE_FILE="$3"; fi
if [[ "$#" -ge 4 ]]; then OUTPUT="$4"; fi

if [[ "$(env | grep -c '^FACESIGN_CONFIRM_MODEL_LICENSE=YES$' || true)" -eq 0 ]]; then
  echo "ERROR: model redistribution was not confirmed." >&2
  echo "Only include weights when their licence permits your intended school use and transfer." >&2
  echo "Set FACESIGN_CONFIRM_MODEL_LICENSE=YES only after confirming those rights." >&2
  exit 2
fi
if [[ ! -x "$APP_DIR/FaceSign" ]]; then
  echo "ERROR: FaceSign executable not found in $APP_DIR" >&2
  exit 1
fi
if [[ -z "$DEB_DIR" || ! -d "$DEB_DIR" ]]; then
  echo "Usage: FACESIGN_CONFIRM_MODEL_LICENSE=YES $0 APP_DIR DEB_DIR MODEL_LICENSE.txt [OUTPUT.tar.gz]" >&2
  exit 1
fi
if [[ -z "$LICENSE_FILE" || ! -f "$LICENSE_FILE" ]]; then
  echo "ERROR: provide the exact model licence/authorization text as MODEL_LICENSE.txt." >&2
  exit 1
fi

DEBS=("$DEB_DIR"/*.deb)
if [[ "${#DEBS[@]}" -eq 0 || ! -f "${DEBS[0]}" ]]; then
  echo "ERROR: no offline DEB packages found in $DEB_DIR" >&2
  exit 1
fi

MODEL_SOURCE="$APP_DIR/models/by"
SCRFD_SOURCE="$MODEL_SOURCE/scrfd_10g.onnx"
ARCFACE_SOURCE="$MODEL_SOURCE/arcface_w600k_r50.onnx"
if [[ ! -f "$SCRFD_SOURCE" && -f "$MODEL_SOURCE/det_10g.onnx" ]]; then
  SCRFD_SOURCE="$MODEL_SOURCE/det_10g.onnx"
fi
if [[ ! -f "$ARCFACE_SOURCE" && -f "$MODEL_SOURCE/w600k_r50.onnx" ]]; then
  ARCFACE_SOURCE="$MODEL_SOURCE/w600k_r50.onnx"
fi
if [[ ! -f "$SCRFD_SOURCE" || ! -f "$ARCFACE_SOURCE" ]]; then
  echo "ERROR: SCRFD and ArcFace model files are both required." >&2
  exit 1
fi

SCRIPT_DIR="$(cd -- "$(dirname -- "$0")" && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT
BUNDLE="$TMP_DIR/bundle"
mkdir -p "$BUNDLE/runtime/facesign" "$BUNDLE/debs"

echo "[1/5] Copying FaceSign runtime (excluding local databases, logs and ZIP archives)..."
tar -C "$APP_DIR" \
  --exclude='*.zip' --exclude='*.db' --exclude='*.log' --exclude='*.pid' \
  --exclude='tls' --exclude='data' \
  -cf - . | tar -C "$BUNDLE/runtime/facesign" -xf -

echo "[2/5] Adding authorized identity models..."
mkdir -p "$BUNDLE/runtime/facesign/models/by"
install -m 644 "$SCRFD_SOURCE" "$BUNDLE/runtime/facesign/models/by/scrfd_10g.onnx"
install -m 644 "$ARCFACE_SOURCE" "$BUNDLE/runtime/facesign/models/by/arcface_w600k_r50.onnx"
install -m 644 "$LICENSE_FILE" "$BUNDLE/runtime/facesign/models/by/MODEL_LICENSE.txt"
(
  cd "$BUNDLE/runtime/facesign/models/by"
  sha256sum scrfd_10g.onnx arcface_w600k_r50.onnx > SHA256SUMS
  sha256sum -c SHA256SUMS
)

echo "[3/5] Copying offline DEB packages..."
cp -a "$DEB_DIR"/. "$BUNDLE/debs/"
(
  cd "$BUNDLE/debs"
  rm -f SHA256SUMS packages.tsv
  for package in *.deb; do
    printf "%s\t" "$package"
    dpkg-deb -f "$package" Package Version Architecture | tr "\n" "\t"
    printf "\n"
  done > packages.tsv
  sha256sum *.deb > SHA256SUMS
)

echo "[4/5] Adding installer and documentation..."
cp "$SCRIPT_DIR/install.sh" "$BUNDLE/install.sh"
cp "$SCRIPT_DIR/README.md" "$BUNDLE/README-OFFLINE.md"
chmod 755 "$BUNDLE/install.sh"
(cd "$BUNDLE" && find . -type f ! -name MANIFEST.sha256 -print0 | sort -z | xargs -0 sha256sum > MANIFEST.sha256)

echo "[5/5] Creating full offline archive..."
mkdir -p "$(dirname "$OUTPUT")"
tar -czf "$OUTPUT" -C "$BUNDLE" .
sha256sum "$OUTPUT"
echo "Full offline package created: $OUTPUT"
