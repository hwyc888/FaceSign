#!/usr/bin/env bash
set -euo pipefail
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh"

if [[ ! -f "$FACESIGN_PID_FILE" ]]; then
  echo "FaceSign is not running (PID file not found)."
  exit 0
fi
pid="$(cat "$FACESIGN_PID_FILE" 2>/dev/null || true)"
if ! facesign_pid_matches "$pid"; then
  rm -f "$FACESIGN_PID_FILE"
  echo "No matching FaceSign process; removed stale PID file."
  exit 0
fi

kill -TERM "$pid"
for attempt in $(seq 1 20); do
  if ! kill -0 "$pid" 2>/dev/null; then
    rm -f "$FACESIGN_PID_FILE"
    echo "FaceSign stopped gracefully."
    exit 0
  fi
  sleep 1
done

echo "FaceSign did not stop within 20 seconds; sending SIGKILL." >&2
kill -KILL "$pid" 2>/dev/null || true
rm -f "$FACESIGN_PID_FILE"
