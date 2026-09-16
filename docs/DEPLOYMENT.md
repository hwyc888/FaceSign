# Deployment

## 1. Platform packages

GitHub Actions publishes two self-contained packages:

- `facesign-linux-amd64.tar.gz`
- `facesign-windows-amd64.zip`

Each package includes the native FaceSign server, the platform service installer, and the matching CompreFace deployment script. CompreFace still runs in Docker, so install and start Docker Engine + Compose on Linux or Docker Desktop on Windows before running the installer. The official CompreFace image requires an x86 CPU with AVX support.

The installer performs the complete local setup:

1. installs FaceSign as a system service;
2. downloads and starts CompreFace 1.2.0;
3. creates or reuses the `FaceSign` application and `FaceSign Recognition` service;
4. writes the generated recognition API key into FaceSign's protected configuration;
5. starts FaceSign and checks both the web server and face-recognition API.

Generated CompreFace administrator credentials remain on the server in the protected CompreFace install directory. Back up that file with the rest of the server configuration. Re-running the installer is idempotent and preserves existing CompreFace volumes and FaceSign data.

## 2. Linux

Extract the Linux package and run:

```sh
tar -xzf facesign-linux-amd64.tar.gz
sudo ./install.sh
```

FaceSign is installed to `/opt/facesign`, its SQLite database defaults to `/var/lib/facesign/facesign.db`, and configuration is stored at `/etc/facesign/facesign.env`.

To install only the management platform and intentionally leave face recognition disabled:

```sh
sudo ./install.sh --skip-face-engine
```

## 3. Windows

Extract the Windows package, open PowerShell as Administrator, and run:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\install-service.ps1
```

FaceSign is installed as an automatic Windows service. Configuration is stored at `C:\Program Files\FaceSign\facesign.env`, and data defaults to `C:\ProgramData\FaceSign\facesign.db`.

To install only the management platform and intentionally leave face recognition disabled:

```powershell
.\install-service.ps1 -SkipFaceEngine
```

## 4. Existing or remote CompreFace

Administrators can change the provider, service URL, API key, similarity threshold, and detection threshold from **人脸识别设置**. Saving applies the settings immediately and persists them in the FaceSign database; the API key is never returned to the browser. Use **检测服务** to verify both reachability and the API key.

Environment variables remain available as first-run defaults:

```text
FACESIGN_FACE_PROVIDER=compreface
FACESIGN_COMPREFACE_URL=http://127.0.0.1:8000
FACESIGN_COMPREFACE_API_KEY=<recognition-service-api-key>
```

## 5. HTTPS for browser camera access

A classroom browser normally needs a secure context to use `getUserMedia`. For anything other than localhost, deploy HTTPS.

FaceSign can terminate TLS directly:

```text
FACESIGN_TLS_CERT=/path/to/server.crt
FACESIGN_TLS_KEY=/path/to/server.key
FACESIGN_COOKIE_SECURE=true
```

Alternatively place Caddy, Nginx, or IIS in front of FaceSign and proxy to `127.0.0.1:8080`.

## 6. First run

Open the server URL in a browser and create the initial FaceSign administrator. Then configure teachers, classes, students, courses, enrollment, and schedules. Before any camera permission is requested, face enrollment checks the configured service and the kiosk checks live service readiness. If it is disabled, unreachable, or has an invalid key, the page stays blocked and displays a specific Chinese message.

Classroom devices open `/kiosk`, enter the exact classroom name from the schedule and the kiosk access key from `facesign.env`, then start the camera.
