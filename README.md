# FaceSign\n\nFaceSign is a compact school face-attendance application written in Go for Windows.\n\n## Design goals\n\n- One application process: web UI, SQLite, face detection, face feature extraction, matching and attendance.\n- No Python runtime.\n- No Docker.\n- No GPU requirement; CPU inference is the default.\n- Browser camera UI is embedded in the Go executable.\n- Portable Windows release built by GitHub Actions.\n\n## Face recognition stack\n\n- YuNet ONNX model for face detection.\n- SFace ONNX model for face embeddings.\n- ONNX Runtime CPU for inference.\n- `go-onnxface` Go API for YuNet/SFace integration.\n\nONNX Runtime is distributed as `onnxruntime.dll` beside `FaceSign.exe`. This is a native library, not a Python dependency.\n\n## Features\n\n- Add and delete students.\n- Class and student number fields.\n- Browser camera enrollment.\n- One face template per student, replaceable by re-enrollment.\n- Face recognition and daily check-in.\n- One check-in record per student per day.\n- Attendance history by date.\n- Embedded responsive left-sidebar UI.\n- SQLite data stored locally.\n- Windows startup installation through Task Scheduler.\n\n## Windows program package\n\nThe generated Windows program package intentionally does **not** contain ONNX model files.
> Program artifacts do not include ONNX models. For a normal Windows deployment, run `scripts\install.ps1`; it will reuse installed models or fetch the pinned models once on a fresh machine.
 The fixed models are stored once in the repository at commit `5f4dbed1d5e78b95b9af95f492f363d654bb01d9`.\n\n```text\nFaceSign.exe\nonnxruntime.dll\nscripts/\n  install.ps1\n  uninstall.ps1\nREADME.md\n```\n\nThe installer keeps existing valid models in `C:\\ProgramData\\FaceSign\\models`. On a fresh installation it downloads the two pinned repository models once and verifies their SHA-256 checksums. Future program upgrades reuse them and do not download them again.\n\nDouble-click `FaceSign.exe`. The server listens on port 8080 and opens `http://127.0.0.1:8080/` automatically.\n\nThe default listen address is `0.0.0.0:8080`, so the same process can also be reached from trusted LAN computers with `http://<server-ip>:8080/`.\n\nIf startup fails, check `facesign-error.log` beside the executable. If that directory is not writable, the fallback log is `%TEMP%\\facesign-error.log`.\n\n## Windows startup installation\n\nRun PowerShell as Administrator from the extracted release directory:\n\n```powershell\nSet-ExecutionPolicy -Scope Process Bypass\n.\\scripts\\install.ps1\n```\n\nThe installer copies the package to `C:\\ProgramData\\FaceSign`, creates a startup scheduled task, and adds a Windows Firewall rule for Domain/Private networks on the configured port. Data is kept in `C:\\ProgramData\\FaceSign\\data`.\n\n## Camera note\n\nBrowser camera APIs work on `http://127.0.0.1` / `localhost`. Remote LAN browsers normally require HTTPS for camera access. Remote LAN clients can still use management pages over HTTP, but camera enrollment/recognition should be performed on the FaceSign Windows PC until HTTPS is added.\n\n## Security note\n\nThe current lightweight build does not yet include administrator authentication. Use LAN access only on a trusted school network. Face embeddings are biometric data; protect the Windows host and database and follow the school's consent, retention and deletion requirements.\n\n## Local development\n\n```sh\ngo test ./...\ngo vet ./...\n```\n

## Upgrading an existing Windows installation

If FaceSign was previously installed as a scheduled task, do not just double-click a newly downloaded executable while the old task is still running. Run the new installer as Administrator:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\scripts\install.ps1
```

The installer stops the old scheduled task and any old `FaceSign.exe` process, replaces the files, starts the new build, verifies `/api/version`, and opens a cache-busting local URL.

For diagnostics:

- `http://127.0.0.1:8080/api/version` shows the running build version.
- `C:\ProgramData\FaceSign\data\facesign-startup.log` shows the version, PID and listening address.
- A startup failure writes `facesign-error.log`.


## Pinned face models

The model binaries are committed once and are independent from normal program builds:

- Model commit: `5f4dbed1d5e78b95b9af95f492f363d654bb01d9`
- YuNet: `models/face_detection_yunet_2023mar.onnx`
- SFace: `models/face_recognition_sface_2021dec.onnx`
- Checksums: `models/SHA256SUMS`

This keeps every later Windows program artifact small. If a target PC must be installed offline, download those two model files once from the pinned commit and place them in a `models` folder beside the extracted program package before running `scripts\install.ps1`.
