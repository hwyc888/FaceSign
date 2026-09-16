# Deployment

## 1. Recommended face engine: local CPU, no GPU or Docker

FaceSign's default deployment target is the **Local CPU Engine** included under `deploy/face-engine`. It uses OpenCV YuNet for face detection and SFace for face embeddings. Inference runs on CPU only; CUDA, a discrete GPU, Docker and Docker Desktop are not required.

The engine implements the small recognition API used by FaceSign and listens on `127.0.0.1:18081` by default. Face samples are stored in a separate SQLite database owned by the face engine.

### Windows

Open PowerShell as Administrator from the release package's `face-engine` directory:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\install-localcpu.ps1
```

The script does not require a preinstalled Python. It downloads an isolated Python 3.11 embeddable runtime, installs CPU-only OpenCV/FastAPI dependencies, downloads the YuNet/SFace ONNX models, registers the `FaceSignFaceEngine` startup task and checks `http://127.0.0.1:18081/health`.

Default engine data is stored under:

```text
C:\ProgramData\FaceSign\face-engine\data
```

Uninstall while preserving face data:

```powershell
.\uninstall-localcpu.ps1
```

Use `-RemoveData` only when you intentionally want to delete the collected face embeddings.

### Linux

```sh
chmod +x ./install-localcpu.sh
./install-localcpu.sh
```

The script creates an isolated Python virtual environment, installs CPU-only dependencies and registers `facesign-face-engine.service` with systemd. Docker and GPU drivers are not required.

### FaceSign admin settings

After the local engine is running, sign in as an administrator and open **Face Recognition**:

```text
Engine: Local CPU Engine
Service URL: http://127.0.0.1:18081
API Key: leave blank
Similarity threshold: 0.72 recommended starting point
Detection threshold: 0.80
```

Save the settings and click **Check Service**. A healthy installation reports that the local CPU engine is running and that GPU/Docker are not required.

## 2. Optional external CompreFace provider

If a school already operates CompreFace on a protected internal server, FaceSign can still use it. Select **CompreFace** in the administrator Face Recognition page, enter its URL and Face Recognition Service API key, then click **Check Service**.

Environment variables remain available only as bootstrap defaults:

```text
FACESIGN_FACE_PROVIDER=compreface
FACESIGN_COMPREFACE_URL=http://127.0.0.1:8000
FACESIGN_COMPREFACE_API_KEY=<recognition-service-api-key>
```

Normal production administration should use the web UI; saved face-engine settings are stored in FaceSign's SQLite database and apply immediately.

## 3. HTTPS for browser camera access

A classroom browser normally needs a secure context to use `getUserMedia`. For anything other than localhost, deploy HTTPS.

FaceSign can terminate TLS directly:

```text
FACESIGN_TLS_CERT=/path/to/server.crt
FACESIGN_TLS_KEY=/path/to/server.key
FACESIGN_COOKIE_SECURE=true
```

Alternatively place Caddy/Nginx/IIS in front of FaceSign and proxy to `127.0.0.1:8080`.

## 4. Linux FaceSign server

Download the `facesign-linux-amd64` artifact, then from the checked-out repository:

```sh
sudo ./deploy/linux/install.sh /path/to/facesign-linux-amd64
sudo nano /etc/facesign/facesign.env
sudo systemctl restart facesign
```

Data defaults to `/var/lib/facesign/facesign.db`.

## 5. Windows FaceSign server

Download `facesign-windows-amd64.exe`. Open PowerShell as Administrator:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\deploy\windows\install-service.ps1 -Binary .\facesign-windows-amd64.exe
```

The Go executable contains native Windows Service support. Configuration is stored beside the installed executable at `C:\Program Files\FaceSign\facesign.env`, and data defaults to `C:\ProgramData\FaceSign\facesign.db`.

The Windows FaceSign service and the local CPU face engine are independent services. Neither requires Docker.

## 6. First run

Open the server URL in a browser. The first page creates the initial administrator. Then configure teachers, classes, students, courses, course enrollment and schedules. Open **Face Recognition**, deploy/configure the local CPU engine, verify its status, then use the student list's face-capture button to register 2-3 samples per student.

Classroom devices open `/kiosk`, enter the exact classroom name from the schedule and the kiosk access key from `facesign.env`, then start the camera.
