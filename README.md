# FaceSign\n\nFaceSign is a compact school face-attendance application written in Go for Windows.\n\n## Design goals\n\n- One application process: web UI, SQLite, face detection, face feature extraction, matching and attendance.\n- No Python runtime.\n- No Docker.\n- No GPU requirement; CPU inference is the default.\n- Browser camera UI is embedded in the Go executable.\n- Portable Windows release built by GitHub Actions.\n\n## Face recognition stack\n\n- YuNet ONNX model for face detection.\n- SFace ONNX model for face embeddings.\n- anti-spoof-mn3 ONNX model for passive RGB liveness / presentation-attack screening.\n- Multi-frame liveness voting before attendance is written.\n- ONNX Runtime CPU for inference.\n- `go-onnxface` Go API for YuNet/SFace integration.\n\nONNX Runtime is distributed as `onnxruntime.dll` beside `FaceSign.exe`. This is a native library, not a Python dependency.\n\n## Features\n\n- Add and delete students.\n- Class and student number fields.\n- Browser camera enrollment.\n- One face template per student, replaceable by re-enrollment.\n- Face recognition and daily check-in.\n- One check-in record per student per day.\n- Attendance history by date.\n- Embedded responsive left-sidebar UI.\n- SQLite data stored locally.\n- Windows startup installation through Task Scheduler.\n\n## Windows program package\n\nThe generated Windows program package intentionally does **not** contain ONNX model files.
> Program artifacts do not include ONNX models. For a normal Windows deployment, run `scripts\install.ps1`; it will reuse installed models or fetch the pinned models once on a fresh machine.
 The fixed models are stored once in the repository at commit `de5287c66e9e37e9f804686bf63f5a0974f68f72`.\n\n```text\nFaceSign.exe\nonnxruntime.dll\nscripts/\n  install.ps1\n  uninstall.ps1\nREADME.md\n```\n\nThe installer keeps existing valid models in `C:\\ProgramData\\FaceSign\\models`. On a fresh installation it downloads the three pinned repository models once and verifies their SHA-256 checksums. Future program upgrades reuse them and do not download them again.

The executable also resolves models itself. Therefore the model-free Windows ZIP can be started directly: FaceSign first reuses a valid existing model cache (including `C:\\ProgramData\\FaceSign\\models`), and if a model is missing it downloads only that pinned model once. If the program directory is not writable, the per-user cache is used instead.\n\nDouble-click `FaceSign.exe`. The server resolves or downloads any missing pinned models, listens on port 8080, and opens `http://127.0.0.1:8080/` automatically.\n\nThe default listen address is `0.0.0.0:8080`, so the same process can also be reached from trusted LAN computers with `http://<server-ip>:8080/`.\n\nIf startup fails, check `facesign-error.log` beside the executable. If that directory is not writable, the fallback log is `%TEMP%\\facesign-error.log`.\n\n## Windows startup installation\n\nRun PowerShell as Administrator from the extracted release directory:\n\n```powershell\nSet-ExecutionPolicy -Scope Process Bypass\n.\\scripts\\install.ps1\n```\n\nThe installer copies the package to `C:\\ProgramData\\FaceSign`, creates a startup scheduled task, and adds a Windows Firewall rule for Domain/Private networks on the configured port. Data is kept in `C:\\ProgramData\\FaceSign\\data`.\n\n## Camera note\n\nBrowser camera APIs work on `http://127.0.0.1` / `localhost`. Remote LAN browsers normally require HTTPS for camera access. Remote LAN clients can still use management pages over HTTP, but camera enrollment/recognition should be performed on the FaceSign Windows PC until HTTPS is added.\n\n## Security note\n\nFace check-in uses passive RGB anti-spoof inference plus multi-frame voting before attendance is accepted. This reduces simple printed-photo and screen-replay attacks without requiring students to blink or turn their head, but RGB-only liveness cannot guarantee rejection of every sophisticated replay or 3D-mask attack.\n\nThe current lightweight build does not yet include administrator authentication. Use LAN access only on a trusted school network. Face embeddings are biometric data; protect the Windows host and database and follow the school's consent, retention and deletion requirements.\n\n## Local development\n\n```sh\ngo test ./...\ngo vet ./...\n```\n

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

- Model commit: `de5287c66e9e37e9f804686bf63f5a0974f68f72`
- YuNet: `models/face_detection_yunet_2023mar.onnx`
- SFace: `models/face_recognition_sface_2021dec.onnx`
- Passive liveness: `models/anti-spoof-mn3.onnx`
- Checksums: `models/SHA256SUMS`

This keeps every later Windows program artifact small. If a target PC must be installed offline, download those three model files once from the pinned commit and place them in a `models` folder beside the extracted program package before running `scripts\install.ps1`.
