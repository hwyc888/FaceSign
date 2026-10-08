# FaceSign\n\nFaceSign is a compact school face-attendance application written in Go for Windows.\n\n## Design goals\n\n- One application process: web UI, SQLite, face detection, face feature extraction, matching and attendance.\n- No Python runtime.\n- No Docker.\n- No GPU requirement; CPU inference is the default.\n- Browser camera UI is embedded in the Go executable.
- A saved **FaceSign host / USB camera** is captured on the Windows computer running FaceSign through the bundled FFmpeg DirectShow input. Remote browsers (including phones/tablets) therefore see the server computer's USB camera instead of silently switching to the remote device camera.\n- Portable Windows release built by GitHub Actions.\n\n## Face recognition stack\n\n- YuNet ONNX model for face detection.\n- SFace ONNX model for face embeddings.\n- anti-spoof-mn3 ONNX model for passive RGB liveness / presentation-attack screening.\n- Multi-frame liveness voting before attendance is written.\n- ONNX Runtime CPU for inference.\n- `go-onnxface` Go API for YuNet/SFace integration.\n\nONNX Runtime is distributed as `onnxruntime.dll` beside `FaceSign.exe`. This is a native library, not a Python dependency.\n\n## Features\n\n- Add and delete students.\n- Class and student number fields.\n- Browser camera enrollment.\n- One face template per student, replaceable by re-enrollment.\n- Face recognition and daily check-in.\n- One check-in record per student per day.\n- Attendance history by date.\n- Embedded responsive left-sidebar UI.\n- SQLite data stored locally.\n- Portable-first Windows operation: unzip and run directly; optional startup registration through the management tool.\n\n## Windows program package

Each successful Windows build now publishes **two release artifacts** from the same `FaceSign.exe` build:

- `facesign-windows-amd64-full`: complete offline **portable package**. Unzip it anywhere with write permission and run `FaceSign.exe` directly; no installation is required. It contains the four pinned ONNX models and is recommended for a new PC, offline deployment, or recovery.
- `facesign-windows-amd64-lite`: lightweight upgrade package. It does not contain ONNX models and is recommended when the existing FaceSign directory already contains valid models.

Both packages contain the same program and native runtime:

```text
FaceSign.exe
FaceSignManager.exe
onnxruntime.dll
scripts/
  install.ps1
  upgrade.ps1
  uninstall.ps1
  install-client-ca.ps1
README.md
```

The full package additionally contains:

```text
models/
  face_detection_yunet_2023mar.onnx
  face_recognition_sface_2021dec.onnx
  anti-spoof-mn3.onnx
  yolox_nano.onnx
  SHA256SUMS
  ...
```

The installer always uses this order:

1. Reuse valid models already present in the selected/current FaceSign directory.
2. If the extracted package contains valid local models, copy only the missing ones.
3. Only if a required model is still missing or invalid, download that pinned model once and verify its SHA-256 checksum.

This means normal upgrades can use the lightweight package without downloading the models again. A fresh machine can use the full package completely offline.

The executable itself still resolves models using the same pinned model set. Double-clicking the full package works offline. The lightweight package can also start directly when a valid existing model cache is available; otherwise it downloads only missing pinned models once.

FaceSign now listens on HTTP `0.0.0.0:8080` and HTTPS `0.0.0.0:8443`. The Windows installer enables HTTP-to-HTTPS redirect while keeping `http://<server-ip>:8080/facesign-root-ca.crt` available for initial client trust setup.

If startup fails, check `facesign-error.log` beside the executable. If that directory is not writable, the fallback log is `%TEMP%\\facesign-error.log`.

## Portable use and optional Windows startup registration

**Portable use is the default.** Extract the full package to any writable folder (for example `D:\\FaceSign`) and double-click `FaceSign.exe`. The program, database, TLS identity, models and logs stay under that same folder.

If FaceSign should run at boot, open `FaceSignManager.exe` and click **安装/注册本目录**. The manager requests administrator rights only for the Windows scheduled task, firewall and certificate operations. It registers the **current extracted directory in place** and does not copy the application to `C:\\ProgramData` or another C-drive installation directory.

The PowerShell installer remains available for diagnostics/automation, but its default target is also the package directory itself.\n\n## FaceSign management tool

The Windows package includes `FaceSignManager.exe`. It runs directly from the extracted folder. When **安装/注册本目录** is used, the same folder is registered as the FaceSign startup location and a **FaceSign 管理工具** shortcut is added to the Windows Start menu.

The manager automatically requests administrator rights because startup-task, firewall and machine-certificate changes require elevation. The FaceSign startup task itself now runs as the **currently signed-in Windows user** with an interactive logon token so Windows DirectShow/USB cameras remain visible. It provides:

- **安装/注册本目录** without copying program files to another directory.
- Start, stop and restart the registered FaceSign task. Repeated Start/Stop clicks are idempotent: starting an already-running service or stopping an already-stopped service returns immediately instead of re-running Task Scheduler commands. The native Win32 GUI message loop is pinned to its creating OS thread so background work cannot strand the window on a different thread.
- **Upgrade FaceSign** directly from the management window: choose the newly extracted release directory, confirm once, and the manager closes itself, runs `scripts\upgrade.ps1`, then reopens after the in-place upgrade finishes. Directory selection and every service/shell operation run off the GUI thread so the management window stays responsive. The database, face data, TLS identity, service arguments and startup state are preserved.
- Stop the scheduled task first, wait for its owned process to exit, then terminate only remaining `FaceSign.exe` PIDs individually. The manager no longer uses `taskkill /T /IM`, so an FFmpeg/decoder child-process error cannot falsely report that FaceSign itself failed to stop. Older scheduled tasks are also normalized to `MultipleInstances=IgnoreNew` during upgrade.
- Enable or disable startup without deleting the task.
- Open the FaceSign management webpage.
- Show the current PID, HTTP/HTTPS listen addresses, running build version and persistent root-CA expiry.
- Open the startup log and installation directory.

Stopping FaceSign does **not** disable startup. Use **关闭开机启动** separately if FaceSign should also stay stopped after the next reboot. Because host USB cameras require the interactive Windows desktop session, the registered task starts at that user's logon rather than in SYSTEM Session 0; FaceSign therefore starts after that Windows user signs in. The registered task passes `--background=true`, so the long-running FaceSign server does not leave a black console window on the desktop. Portable double-click mode remains unchanged and may show its console for diagnostics.

## HTTPS and browser camera access

FaceSign now creates a persistent private root CA the first time it starts. The root CA is valid for 50 years and is preserved across normal upgrades and uninstall/reinstall. Server certificates are renewed automatically from the same root CA before expiry or whenever the server hostname/IP SAN set changes, so clients keep trusting the server without reinstalling the CA.

After optional Windows startup registration, the TLS identity stays under the selected FaceSign directory's `tls` folder. The installer restricts that directory to SYSTEM and local Administrators and imports the root CA into the server's LocalMachine trust store.

On each Windows browser client, run:

```powershell
.\\scripts\\install-client-ca.ps1 -Server <FaceSign-server-IP>
```

This installs trust for the current Windows user without administrator rights. Use `-Machine` from an Administrator PowerShell window to trust FaceSign for all users on that client. Then open `https://<server-ip>:8443/`; Edge/Chrome will treat the page as a secure context and can call that client's own USB/built-in camera.

If the server is accessed through an extra DNS name or a NAT/public IP not assigned directly to a server interface, install with `-TLSHosts "facesign.school.example,203.0.113.10"` so those names are included in the HTTPS certificate.\n\n## Security note\n\nFace check-in uses passive RGB anti-spoof inference plus multi-frame voting before attendance is accepted. This reduces simple printed-photo and screen-replay attacks without requiring students to blink or turn their head, but RGB-only liveness cannot guarantee rejection of every sophisticated replay or 3D-mask attack.\n\nThe current lightweight build does not yet include administrator authentication. Use LAN access only on a trusted school network. Face embeddings are biometric data; protect the Windows host and database and follow the school's consent, retention and deletion requirements.\n\n## Local development\n\n```sh\ngo test ./...\ngo vet ./...\n```\n

## Upgrading an existing Windows installation

Normal upgrades preserve FaceSign data and service arguments while refreshing the scheduled task for the currently signed-in Windows user. From the newly extracted release package, run this as Administrator:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\scripts\upgrade.ps1
```

The upgrade stops the running FaceSign process, replaces only program/runtime files, recreates the startup task as an interactive current-user logon task, and then starts the new build. It preserves custom HTTP/HTTPS/TLS-host arguments, the current startup-enabled/disabled state, the SQLite database, TLS identity and cached models.

`scripts\install.ps1` is now upgrade-aware too: if an existing FaceSign task and installation are detected, rerunning it performs the same in-place update instead of unregistering and recreating the task. Explicitly passing `-Listen`, `-HTTPSListen` or `-TLSHosts` still updates those service arguments when that is intentional.

For diagnostics:

- `https://127.0.0.1:8443/api/version` shows the running build version after the FaceSign root CA has been trusted.
- `data\facesign-startup.log` under the FaceSign program directory shows the version, PID and listening address.
- A startup failure writes `facesign-error.log`.


## Pinned face models

The core face/liveness model binaries are pinned in FaceSign history. The person detector is the official YOLOX-Nano release asset, downloaded by the full-package build/installer and verified by its pinned SHA-256:

- Model commit: `de5287c66e9e37e9f804686bf63f5a0974f68f72`
- YuNet: `models/face_detection_yunet_2023mar.onnx`
- SFace: `models/face_recognition_sface_2021dec.onnx`
- Passive liveness: `models/anti-spoof-mn3.onnx`
- Person detector: official YOLOX-Nano 416 ONNX (`0.1.1rc0`), SHA-256 pinned in `models/SHA256SUMS`
- Checksums: `models/SHA256SUMS`

This keeps every later Windows program artifact small. If a target PC must be installed offline, download those three model files once from the pinned commit and place them in a `models` folder beside the extracted program package before running `scripts\install.ps1`.


## Remote server + classroom Camera Agent

When the FaceSign server is outside the classroom LAN, browser-local USB cameras continue to work from the client browser (use HTTPS for remote browser camera permission). For classroom IP cameras that the central server cannot route to, use the separate `facesign-camera-agent-windows-amd64` artifact.

The Camera Agent runs on a classroom Windows PC, reads the camera locally, and only makes outbound HTTP/HTTPS requests to the central FaceSign server. No inbound port or router port-forward is required. Camera URL/user/password remain in the classroom PC's local `camera-agent.json`; the server stores only the Agent ID, a SHA-256 connection-key hash, and the latest frame in memory.

See `CAMERA_AGENT.md` in the repository or the Camera Agent artifact for deployment steps.


## FaceSign host USB camera from phones and other computers

Camera Management treats **FaceSign主机 / USB摄像头** as a server-side camera source. Device enumeration, connection testing, continuous preview frames and recognition frames all run on the Windows computer that is running `FaceSign.exe`.

This fixes the old browser-local behavior where opening FaceSign from a phone caused `getUserMedia()` to open the phone camera. The browser camera remains only as the temporary fallback when no saved camera exists. Once a host USB camera is saved as the default, desktops, tablets and phones all use the same FaceSign-host camera.

The host USB path uses the bundled FFmpeg DirectShow input and one shared capture process for preview and recognition, because many Windows webcams cannot be opened twice at the same time. Windows webcams are session-scoped, so the optional startup task is registered for the currently signed-in desktop user instead of SYSTEM/Session 0. If an older installation still reports no host USB camera, open the new `FaceSignManager.exe` and run **安装/注册本目录** once to migrate the old task. Also confirm the USB camera is connected and Windows Privacy & security -> Camera allows desktop applications to access the camera.

The web UI also has a phone/tablet layout: tablet navigation becomes horizontal, phone navigation moves to the bottom, forms stack to one column, and data tables render as labeled cards on narrow screens.


### Host USB camera enumeration fallback

FaceSign first enumerates Windows host cameras through FFmpeg DirectShow. If a Windows/FFmpeg combination returns no DirectShow list, FaceSign also checks Windows Plug and Play Camera/Image devices and uses their friendly names as fallback capture candidates. A failed host-USB connection test reports both **FFmpeg DirectShow** and **Windows PnP** status so it is immediately clear whether the camera is missing at the Windows hardware layer or only unavailable to DirectShow.


### Opt-in camera audio

Live camera audio is **off by default**. Clicking **声音：关/开** creates a separate audio-only WebRTC connection, so the existing H.264/H.265 preview path, MJPEG fallback and face-recognition cadence are unchanged while sound is off. The volume slider defaults to 60%.

- RTSP network cameras: FaceSign reads the audio track from the same RTSP stream and converts it to WebRTC Opus only after sound is enabled.
- FaceSign host USB cameras: FaceSign enumerates Windows DirectShow audio inputs only after sound is enabled and prefers an input whose device name best matches the selected USB camera.
- Current-browser cameras and Camera Agent stay video-only to avoid microphone feedback and to preserve their existing behavior.


### Camera microphone association

The sound control now means **the microphone that belongs to the selected camera**, not an arbitrary PC recording input.

- Current-browser camera: after the user enables sound, FaceSign uses the browser's MediaDeviceInfo `groupId` to find an `audioinput` in the same physical device group as the active `videoinput`. If no same-group microphone exists, sound stays off and FaceSign does not fall back to another microphone.
- FaceSign host USB camera: the server only accepts a DirectShow audio input that positively matches the selected USB camera name/device tokens. An unrelated Realtek/laptop microphone is no longer selected as a fallback.
- RTSP camera: sound still comes from the audio track embedded in that camera's RTSP stream.

Audio remains off by default and enabling/disabling it does not change the existing video or face-recognition path.
