# Deployment

## 1. Recommended face engine: native CPU, no Python, Docker or GPU

FaceSign's default face-recognition deployment is the **Native CPU Engine** shipped inside the release package. The engine itself is a Go executable and uses ONNX Runtime CPU with YuNet face detection and SFace embeddings.

The target server does **not** install Python, pip, venv, Docker, Docker Desktop, CUDA or GPU drivers. ONNX Runtime and the two ONNX models are application files carried inside the FaceSign release package.

The engine listens on `127.0.0.1:18081` by default and stores face embeddings in SQLite `faces.db`. The schema is intentionally compatible with the previous local CPU engine, so already-enrolled face samples remain usable after upgrading.

### Windows

Open the release package's `face-engine\windows` directory and run:

```powershell
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

The installer copies the already-built native executable, ONNX Runtime CPU DLL, VC runtime app-local DLLs and models into FaceSign's private installation directory, then registers the native Windows Service:

```text
FaceSignFaceEngine
```

No runtime download or language environment is created on the target machine.

Default face data:

```text
C:\ProgramData\FaceSign\face-engine\data\faces.db
```

Uninstall while preserving enrolled face data:

```powershell
powershell -ExecutionPolicy Bypass -File .\uninstall.ps1
```

Delete biometric data only when intentionally required:

```powershell
powershell -ExecutionPolicy Bypass -File .\uninstall.ps1 -RemoveData
```

### Linux

Open the release package's `face-engine/linux` directory:

```sh
chmod +x ./install.sh
sudo ./install.sh
```

The installer copies the native executable, ONNX Runtime CPU shared library and models to `/opt/facesign/face-engine`, then registers `facesign-face-engine.service` with systemd. It does not install Python or Docker.

Default face data:

```text
/var/lib/facesign/face-engine/data/faces.db
```

### FaceSign admin settings

After the engine is installed, sign in as an administrator and open **Face Recognition**:

```text
Engine: Local Native CPU Engine
Service URL: http://127.0.0.1:18081
API Key: leave blank
Similarity threshold: 0.72 recommended starting point
Detection threshold: 0.80
```

Save and click **Check Service**. A healthy engine reports CPU mode and `gpu_required=false`.

### Moving to a new server

A new server does not need Python/Docker reconstruction. Install the same FaceSign release and copy these persistent databases from the old machine:

```text
FaceSign database: facesign.db
Face engine database: faces.db
```

Then start the FaceSign and FaceSignFaceEngine services. No face re-enrollment is required as long as `faces.db` is migrated with the application data.

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

Download `facesign-linux-amd64`, then from the checked-out repository:

```sh
sudo ./deploy/linux/install.sh /path/to/facesign-linux-amd64
sudo nano /etc/facesign/facesign.env
sudo systemctl restart facesign
```

FaceSign data defaults to `/var/lib/facesign/facesign.db`.

## 5. Windows FaceSign server

Download `facesign-windows-amd64.exe`. Open PowerShell as Administrator:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\deploy\windows\install-service.ps1 -Binary .\facesign-windows-amd64.exe
```

The Go executable contains native Windows Service support. Configuration is stored beside the installed executable at `C:\Program Files\FaceSign\facesign.env`, and FaceSign data defaults to `C:\ProgramData\FaceSign\facesign.db`.

FaceSign and `FaceSignFaceEngine` are separate native Windows services, but both are delivered by the same release package and neither requires Docker or Python.

## 6. First run

Open the server URL in a browser. The first page creates the initial administrator. Configure teachers, classes, students, courses, course enrollment and schedules. Then open **Face Recognition**, install/configure the native CPU engine, verify its status, and register 2-3 face samples per student.

Classroom devices open `/kiosk`, enter the exact classroom name from the schedule and the kiosk access key from `facesign.env`, then start the camera.
