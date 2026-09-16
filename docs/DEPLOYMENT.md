# Deployment

## 1. Face recognition service

Production face matching uses a CompreFace Face Recognition service. Deploy CompreFace on the same server or a protected internal host, create a Face Recognition service in its UI, and copy its API key.

Set in `facesign.env`:

```text
FACESIGN_FACE_PROVIDER=compreface
FACESIGN_COMPREFACE_URL=http://127.0.0.1:8000
FACESIGN_COMPREFACE_API_KEY=<recognition-service-api-key>
```

FaceSign uses CompreFace's recognition face collection to add student samples and `/api/v1/recognition/recognize` to identify kiosk captures.

## 2. HTTPS for browser camera access

A classroom browser normally needs a secure context to use `getUserMedia`. For anything other than localhost, deploy HTTPS.

FaceSign can terminate TLS directly:

```text
FACESIGN_TLS_CERT=/path/to/server.crt
FACESIGN_TLS_KEY=/path/to/server.key
FACESIGN_COOKIE_SECURE=true
```

Alternatively place Caddy/Nginx/IIS in front of FaceSign and proxy to `127.0.0.1:8080`.

## 3. Linux

Download the `facesign-linux-amd64` artifact, then from the checked-out repository:

```sh
sudo ./deploy/linux/install.sh /path/to/facesign-linux-amd64
sudo nano /etc/facesign/facesign.env
sudo systemctl restart facesign
```

Data defaults to `/var/lib/facesign/facesign.db`.

## 4. Windows

Download `facesign-windows-amd64.exe`. Open PowerShell as Administrator:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\deploy\windows\install-service.ps1 -Binary .\facesign-windows-amd64.exe
```

The executable contains native Windows Service support. Configuration is stored beside the installed executable at `C:\Program Files\FaceSign\facesign.env`, and data defaults to `C:\ProgramData\FaceSign\facesign.db`.

## 5. First run

Open the server URL in a browser. The first page creates the initial administrator. Then configure teachers, classes, students, courses, course enrollment and schedules. Use the student list's face-capture button to register 2-3 samples per student.

Classroom devices open `/kiosk`, enter the exact classroom name from the schedule and the kiosk access key from `facesign.env`, then start the camera.
