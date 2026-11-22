#!/usr/bin/env bash
set -Eeuo pipefail

if ! command -v docker >/dev/null 2>&1; then
  echo "ERROR: Docker is required to prepare a clean Ubuntu 24.04 dependency set." >&2
  exit 1
fi

DEST_DIR="$PWD/facesign-ubuntu2404-debs"
if [[ "$#" -gt 0 ]]; then DEST_DIR="$1"; fi
mkdir -p "$DEST_DIR"
DEST_DIR="$(cd "$DEST_DIR" && pwd)"

echo "Downloading Ubuntu 24.04 AMD64 DEB packages into: $DEST_DIR"
docker run --rm --platform linux/amd64 \
  -v "$DEST_DIR:/out" \
  ubuntu:24.04 \
  bash -euxo pipefail -c '
    export DEBIAN_FRONTEND=noninteractive
    apt-get update
    apt-get install --download-only --reinstall --no-install-recommends -y \
      ca-certificates curl ffmpeg libstdc++6 libgomp1 libgcc-s1
    find /var/cache/apt/archives -maxdepth 1 -type f -name "*.deb" -exec cp -v {} /out/ \;
    shopt -s nullglob
    packages=(/out/*.deb)
    if [[ "${#packages[@]}" -eq 0 ]]; then
      echo "No DEB packages were downloaded." >&2
      exit 1
    fi
    cd /out
    for package in *.deb; do
      printf "%s\t" "$package"
      dpkg-deb -f "$package" Package Version Architecture | tr "\n" "\t"
      printf "\n"
    done > packages.tsv
    sha256sum *.deb > SHA256SUMS
  '

echo "Offline DEB set prepared."
echo "Packages: $(find "$DEST_DIR" -maxdepth 1 -type f -name "*.deb" | wc -l)"
