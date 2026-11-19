#!/usr/bin/env bash
set -euo pipefail
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh"

if [[ ! -x "$FACESIGN_BIN" ]]; then
  echo "FaceSign executable not found or not executable: $FACESIGN_BIN" >&2
  exit 1
fi
if [[ ! -f "$FACESIGN_APP_DIR/libonnxruntime.so.1.26.0" ]]; then
  echo "Missing libonnxruntime.so.1.26.0 beside FaceSign." >&2
  exit 1
fi
for model in scrfd_10g.onnx arcface_w600k_r50.onnx SHA256SUMS; do
  if [[ ! -f "$FACESIGN_BY_MODEL_DIR/$model" ]]; then
    echo "Missing authorized face model: $FACESIGN_BY_MODEL_DIR/$model" >&2
    echo "Read models/by/README.md and place the licensed SCRFD/ArcFace weights and checksum manifest there." >&2
    exit 1
  fi
done
if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required by the startup health check. Install it with: sudo apt-get install curl" >&2
  exit 1
fi
if ! command -v ffmpeg >/dev/null 2>&1; then
  echo "Warning: ffmpeg is not installed; RTSP/network camera decoding may be unavailable." >&2
fi

mkdir -p "$FACESIGN_DATA_DIR" "$FACESIGN_STATE_DIR" "$FACESIGN_TLS_DIR" "$(dirname -- "$FACESIGN_DB_PATH")"

if [[ -f "$FACESIGN_PID_FILE" ]]; then
  old_pid="$(cat "$FACESIGN_PID_FILE" 2>/dev/null || true)"
  if facesign_pid_matches "$old_pid"; then
    echo "FaceSign is already running (PID $old_pid)."
    echo "Health: $FACESIGN_HEALTH_URL"
    exit 0
  fi
  rm -f "$FACESIGN_PID_FILE"
fi

export FACESIGN_BY_MODEL_DIR
nohup env \
  LD_LIBRARY_PATH="$FACESIGN_APP_DIR:${LD_LIBRARY_PATH:-}" \
  FACESIGN_BY_MODEL_DIR="$FACESIGN_BY_MODEL_DIR" \
  "$FACESIGN_BIN" \
  --listen "$FACESIGN_HTTP_ADDR" \
  --https-listen "$FACESIGN_HTTPS_ADDR" \
  --http-redirect=false \
  --tls-dir "$FACESIGN_TLS_DIR" \
  --tls-hosts "${FACESIGN_TLS_HOSTS:-}" \
  --data "$FACESIGN_DB_PATH" \
  --assets "$FACESIGN_APP_DIR" \
  --match-threshold "${FACESIGN_MATCH_THRESHOLD:-0.68}" \
  --detection-threshold "${FACESIGN_DETECTION_THRESHOLD:-0.50}" \
  --open-browser=false \
  >>"$FACESIGN_LOG_FILE" 2>&1 </dev/null &

pid=$!
printf '%s\n' "$pid" > "$FACESIGN_PID_FILE"

for attempt in $(seq 1 45); do
  if ! facesign_pid_matches "$pid"; then
    rm -f "$FACESIGN_PID_FILE"
    echo "FaceSign exited during startup. Recent log output:" >&2
    tail -n 80 "$FACESIGN_LOG_FILE" >&2 || true
    exit 1
  fi
  if curl -fsS --max-time 2 "$FACESIGN_HEALTH_URL" >/dev/null 2>&1; then
    echo "FaceSign started successfully (PID $pid)."
    echo "HTTPS: https://localhost:$FACESIGN_HTTPS_PORT/"
    echo "HTTP health: $FACESIGN_HEALTH_URL"
    echo "Log: $FACESIGN_LOG_FILE"
    exit 0
  fi
  sleep 1
done

echo "FaceSign process is running (PID $pid), but the health endpoint did not respond within 45 seconds." >&2
echo "Recent log output:" >&2
tail -n 80 "$FACESIGN_LOG_FILE" >&2 || true
exit 1
