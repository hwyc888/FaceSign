# FaceSign Ubuntu 24.04 AMD64 Offline Deployment

This package is for **Ubuntu 24.04 AMD64**. It installs the FaceSign Linux runtime, Microsoft ONNX Runtime 1.26.0, FFmpeg, curl, certificates and the required Ubuntu DEB dependencies without accessing the network on the target server.

## Package types

- **Offline runtime artifact from CI:** contains FaceSign, ONNX Runtime, redistributable support models, Ubuntu scripts and offline DEB dependencies. It deliberately does not contain the InsightFace SCRFD/ArcFace identity weights.
- **Full offline archive assembled locally:** can include SCRFD/ArcFace only when you have the exact licence/authorization permitting the intended use and transfer. The local builder requires both an explicit confirmation and a copy of the applicable licence text.

The repository must not publish or redistribute upstream InsightFace pretrained weights whose licence does not permit the intended school attendance use.

## 1. Prepare the DEB set on an internet-connected machine

On a machine with Docker installed:

    chmod +x prepare-debs.sh
    ./prepare-debs.sh ./debs

This uses a clean Ubuntu 24.04 AMD64 container and downloads the dependency closure for:

- ca-certificates
- curl
- ffmpeg
- libstdc++6
- libgomp1
- libgcc-s1

It writes debs/SHA256SUMS and debs/packages.tsv.

## 2. Assemble a full offline archive locally

After downloading/extracting the CI offline runtime artifact, copy your authorized model files into the local FaceSign app directory. The builder accepts either the FaceSign names or the common buffalo_l names:

- scrfd_10g.onnx or det_10g.onnx
- arcface_w600k_r50.onnx or w600k_r50.onnx

You must also have a licence/authorization text file that covers the intended use and transfer. Then run:

    FACESIGN_CONFIRM_MODEL_LICENSE=YES ./build-full-offline-bundle.sh /opt/facesign /path/to/offline-runtime/debs /path/to/MODEL_LICENSE.txt /path/to/FaceSign-Ubuntu-24.04-AMD64-Full-Offline.tar.gz

Do not set the confirmation variable unless you have verified the model rights. SHA-256 proves file integrity, not publisher authenticity or licensing.

## 3. Install on the fully offline target server

Copy the archive to the target Ubuntu 24.04 AMD64 server, then:

    mkdir -p /opt/facesign-offline
    tar -xzf FaceSign-Ubuntu-24.04-AMD64-Full-Offline.tar.gz -C /opt/facesign-offline
    cd /opt/facesign-offline
    chmod +x install.sh
    ./install.sh

The installer:

1. Verifies Ubuntu 24.04 and AMD64.
2. Verifies the offline DEB checksums.
3. Installs DEBs using apt-get --no-download (no network fallback).
4. Copies the FaceSign runtime to /opt/facesign without deleting the existing database or TLS data.
5. Checks FFmpeg, curl, ONNX Runtime and ELF dependencies.
6. Normalizes det_10g.onnx / w600k_r50.onnx to the required FaceSign filenames when needed.
7. Creates SHA256SUMS if it is missing or empty, then verifies both model files.
8. Starts FaceSign and checks its local health endpoint.

If you only have the CI runtime artifact and no identity weights yet, run ./install.sh runtime-only. The runtime is installed, but FaceSign is not started until the authorized SCRFD/ArcFace files and checksum manifest are present.

## FFmpeg and environment variables

FFmpeg is installed from Ubuntu DEB packages and is available as /usr/bin/ffmpeg and /usr/bin/ffprobe. No custom FFMPEG_HOME or profile PATH entry is required.

FaceSign's Ubuntu start.sh sets LD_LIBRARY_PATH to the application directory for its bundled ONNX Runtime libraries. Do not copy system libraries such as libstdc++.so.6 into /usr/lib; install them with the offline DEB packages.

## Data preservation

The installer copies runtime files but does not delete the existing FaceSign data directory, SQLite database, TLS identity, logs or PID/state directories. Back up your existing deployment before replacing binaries.
