# FaceSign\n\nFaceSign is a compact school face-attendance application written in Go for Windows.\n\n## Design goals\n\n- One application process: web UI, SQLite, face detection, face feature extraction, matching and attendance.\n- No Python runtime.\n- No Docker.\n- No GPU requirement; CPU inference is the default.\n- Browser camera UI is embedded in the Go executable.\n- Portable Windows release built by GitHub Actions.\n\n## Face recognition stack\n\n- YuNet ONNX model for face detection.\n- SFace ONNX model for face embeddings.\n- anti-spoof-mn3 ONNX model for passive RGB liveness / presentation-attack screening.\n- Multi-frame liveness voting before attendance is written.\n- ONNX Runtime CPU for inference.\n- `go-onnxface` Go API for YuNet/SFace integration.\n\nONNX Runtime is distributed as `onnxruntime.dll` beside `FaceSign.exe`. This is a native library, not a Python dependency.\n\n## Features\n\n- Add and delete students.\n- Class and student number fields.\n- Browser camera enrollment.\n- One face template per student, replaceable by re-enrollment.\n- Face recognition and daily check-in.\n- One check-in record per student per day.\n- Attendance history by date.\n- Embedded responsive left-sidebar UI.\n- SQLite data stored locally.\n- Windows startup installation through Task Scheduler.\n\n## Windows program package

Each successful Windows build now publishes **two release artifacts** from the same `FaceSign.exe` build:

- `facesign-windows-amd64-full`: complete offline package. It contains the three pinned ONNX models and is recommended for a new PC, offline deployment, or recovery.
- `facesign-windows-amd64-lite`: lightweight upgrade package. It does not contain ONNX models and is recommended when FaceSign is already installed and `C:\\ProgramData\\FaceSign\\models` already contains valid models.

Both packages contain the same program and native runtime:

```text
FaceSign.exe
onnxruntime.dll
scripts/
  install.ps1
  uninstall.ps1
README.md
```

The full package additionally contains:

```text
models/
  face_detection_yunet_2023mar.onnx
  face_recognition_sface_2021dec.onnx
  anti-spoof-mn3.onnx
  SHA256SUMS
  ...
```

The installer always uses this order:

1. Reuse valid models already installed in `C:\\ProgramData\\FaceSign\\models`.
2. If the extracted package contains valid local models, copy only the missing ones.
3. Only if a required model is still missing or invalid, download that pinned model once and verify its SHA-256 checksum.

This means normal upgrades can use the lightweight package without downloading the models again. A fresh machine can use the full package completely offline.

The executable itself still resolves models using the same pinned model set. Double-clicking the full package works offline. The lightweight package can also start directly when a valid existing model cache is available; otherwise it downloads only missing pinned models once.

FaceSign now listens on HTTP `0.0.0.0:8080` and HTTPS `0.0.0.0:8443`. The Windows installer enables HTTP-to-HTTPS redirect while keeping `http://<server-ip>:8080/facesign-root-ca.crt` available for initial client trust setup.

If startup fails, check `facesign-error.log` beside the executable. If that directory is not writable, the fallback log is `%TEMP%\\facesign-error.log`.

## Windows startup installation\n\nRun PowerShell as Administrator from the extracted release directory:\n\n```powershell\nSet-ExecutionPolicy -Scope Process Bypass\n.\\scripts\\install.ps1\n```\n\nThe installer copies the package to `C:\\ProgramData\\FaceSign`, creates a startup scheduled task, and adds a Windows Firewall rule for Domain/Private networks on the configured port. Data is kept in `C:\\ProgramData\\FaceSign\\data`.\n\n## HTTPS and browser camera access

FaceSign now creates a persistent private root CA the first time it starts. The root CA is valid for 50 years and is preserved across normal upgrades and uninstall/reinstall. Server certificates are renewed automatically from the same root CA before expiry or whenever the server hostname/IP SAN set changes, so clients keep trusting the server without reinstalling the CA.

After Windows installation, the TLS identity is stored under `C:\\ProgramData\\FaceSign\\tls`. The installer restricts that directory to SYSTEM and local Administrators and imports the root CA into the server's LocalMachine trust store.

On each Windows browser client, run:

```powershell
.\\scripts\\install-client-ca.ps1 -Server <FaceSign-server-IP>
```

This installs trust for the current Windows user without administrator rights. Use `-Machine` from an Administrator PowerShell window to trust FaceSign for all users on that client. Then open `https://<server-ip>:8443/`; Edge/Chrome will treat the page as a secure context and can call that client's own USB/built-in camera.

If the server is accessed through an extra DNS name or a NAT/public IP not assigned directly to a server interface, install with `-TLSHosts "facesign.school.example,203.0.113.10"` so those names are included in the HTTPS certificate.\n\n## Security note\n\nFace check-in uses passive RGB anti-spoof inference plus multi-frame voting before attendance is accepted. This reduces simple printed-photo and screen-replay attacks without requiring students to blink or turn their head, but RGB-only liveness cannot guarantee rejection of every sophisticated replay or 3D-mask attack.\n\nThe current lightweight build does not yet include administrator authentication. Use LAN access only on a trusted school network. Face embeddings are biometric data; protect the Windows host and database and follow the school's consent, retention and deletion requirements.\n\n## Local development\n\n```sh\ngo test ./...\ngo vet ./...\n```\n

## Upgrading an existing Windows installation

If FaceSign was previously installed as a scheduled task, do not just double-click a newly downloaded executable while the old task is still running. Run the new installer as Administrator:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\scripts\install.ps1
```

The installer stops the old scheduled task and any old `FaceSign.exe` process, replaces the files, starts the new build, verifies `/api/version`, and opens a cache-busting local URL.

For diagnostics:

- `https://127.0.0.1:8443/api/version` shows the running build version after the FaceSign root CA has been trusted.
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


## Remote server + classroom Camera Agent

When the FaceSign server is outside the classroom LAN, browser-local USB cameras continue to work from the client browser (use HTTPS for remote browser camera permission). For classroom IP cameras that the central server cannot route to, use the separate `facesign-camera-agent-windows-amd64` artifact.

The Camera Agent runs on a classroom Windows PC, reads the camera locally, and only makes outbound HTTP/HTTPS requests to the central FaceSign server. No inbound port or router port-forward is required. Camera URL/user/password remain in the classroom PC's local `camera-agent.json`; the server stores only the Agent ID, a SHA-256 connection-key hash, and the latest frame in memory.

See `CAMERA_AGENT.md` in the repository or the Camera Agent artifact for deployment steps.
