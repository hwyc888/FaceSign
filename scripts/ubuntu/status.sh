#!/usr/bin/env bash
set -euo pipefail
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh"

if [[ ! -f "$FACESIGN_PID_FILE" ]]; then
  echo "FaceSign: STOPPED (PID file not found)"
  exit 3
fi
pid="$(cat "$FACESIGN_PID_FILE" 2>/dev/null || true)"
if ! facesign_pid_matches "$pid"; then
  rm -f "$FACESIGN_PID_FILE"
  echo "FaceSign: STOPPED (stale PID file cleaned)"
  exit 3
fi

echo "FaceSign: RUNNING (PID $pid)"
echo "Binary: $FACESIGN_BIN"
echo "Log: $FACESIGN_LOG_FILE"
if command -v curl >/dev/null 2>&1 && curl -fsS --max-time 3 "$FACESIGN_HEALTH_URL" >/dev/null 2>&1; then
  echo "API: HEALTHY ($FACESIGN_HEALTH_URL)"
  exit 0
fi
echo "API: NOT RESPONDING ($FACESIGN_HEALTH_URL)"
exit 1
