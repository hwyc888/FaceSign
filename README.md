# FaceSign

FaceSign is a compact school face-attendance application written in Go for Windows.

## Design goals

- One application process: web UI, SQLite, face detection, face feature extraction, matching and attendance.
- No Python runtime.
- No Docker.
- No GPU requirement; CPU inference is the default.
- Browser camera UI is embedded in the Go executable.
- Portable Windows release built by GitHub Actions.

## Face recognition stack

- YuNet ONNX model for face detection.
- SFace ONNX model for face embeddings.
- ONNX Runtime CPU for inference.
- `go-onnxface` Go API for YuNet/SFace integration.

ONNX Runtime is distributed as `onnxruntime.dll` beside `FaceSign.exe`. This is a native library, not a Python dependency.

## Features in this clean rewrite

- Add and delete students.
- Class and student number fields.
- Browser camera enrollment.
- One face template per student, replaceable by re-enrollment.
- Face recognition and daily check-in.
- One check-in record per student per day.
- Attendance history by date.
- Embedded responsive left-sidebar UI.
- SQLite data stored locally.
- Windows startup installation through Task Scheduler.

## Windowws portable layout

```text
FaceSign.exe
onnxruntime.dll
models/
  face_detection_yunet_2023mar.onnx
  face_recognition_sface_2021dec.onnx
install.ps1
uninstall.ps1
```

Double-clicking the executable is enough for local testing. Open `http://127.0.0.1:8080/`.

For startup installation, run PowerShell as Administrator:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\scripts\install.ps1
```

The installer copies the package to `C:\ProgramData\FaceSign`, creates a startup scheduled task, and keeps the SQLite database in `C:\ProgramData\FaceSign\data`.

## Camera note

Browser camera APIs work on `http://127.0.0.1` / `localhost`. Remote LAN browsers normally require HTTPS for camera access. The intended classroom kiosk is the Windows PC running FaceSign itself; LAN clients may still use the management pages over HTTP.

## Local development

```sh
go test ./...
go vet ./...
```

The runtime DLL and ONNX models are required only when starting the application, not for unit tests.

## Data and privacy

Face embeddings are biometric data. Protect the Windows host and database, restrict access to the management UI, and follow the school's consent, retention and deletion requirements.
