# FaceSign

FaceSign is a Go + SQLite student face-attendance server for school classrooms. Teachers use a browser dashboard, classroom devices use a browser camera kiosk, and attendance is evaluated against course schedules as **准时 / 迟到 / 请假 / 旷课**.

## Current capabilities

- Go server with embedded responsive web UI; no separate web server is required.
- First-run administrator setup, teacher/admin accounts and server-side login sessions.
- Classes, students, courses, course enrollment and weekly schedules.
- Browser camera enrollment for student face samples.
- Browser kiosk at `/kiosk` for classroom face attendance.
- Built-in provider management with a recommended local CPU face engine (OpenCV YuNet + SFace) that does not require GPU, CUDA or Docker; CompreFace remains optional.
- Schedule-based `on_time` / `late` decision with configurable grace period.
- Approved leave records and automatic `absent` finalization at course end.
- Repeated face scans are idempotent per student/session.
- Teacher real-time dashboard using Server-Sent Events (SSE).
- Manual teacher correction of attendance status.
- SQLite WAL mode, busy timeout and constrained writes for concurrent browser clients.
- Argon2id password hashing and opaque cookie sessions.
- Native Linux systemd deployment and native Windows Service execution.
- Optional native HTTPS mode for browser camera secure-context requirements.
- GitHub Actions builds Linux/amd64 and Windows/amd64 server binaries.

## Repository layout

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md). The application is separated into configuration, database, repository, attendance, face provider, realtime, HTTP API and embedded UI packages; `cmd/facesign/main.go` only starts the application.

## Build locally

Requires Go 1.26+.

```sh
go test ./...
go build -o facesign ./cmd/facesign
```

Cross compile:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o dist/facesign-linux-amd64 ./cmd/facesign
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -o dist/facesign-windows-amd64.exe ./cmd/facesign
```

`modernc.org/sqlite` is used so the Windows/Linux builds do not require a C compiler or SQLite DLL.

## Configuration

Copy `config.example.env` to `facesign.env`. FaceSign automatically reads `facesign.env` beside the executable, or use `FACESIGN_ENV_FILE` to point to another file. Real environment variables take precedence.

The server can start with `FACESIGN_FACE_PROVIDER=disabled` while you configure academic data. Face-engine settings are normally managed from **Administrator -> Face Recognition** and stored in SQLite, so `facesign.env` does not need to contain an API key. The recommended Windows/Linux engine is the local CPU service in `deploy/face-engine`; it requires no GPU or Docker. CompreFace is still supported as an optional external provider. See [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md).

## Quick development run

```sh
cp config.example.env facesign.env
# Leave face provider disabled for initial UI setup.
go run ./cmd/facesign
```

Open `http://127.0.0.1:8080/` for the teacher/admin platform. The first run asks you to create the initial administrator.

## Production notes

- Use HTTPS for classroom camera devices; browser camera APIs generally require a secure context outside localhost.
- The local CPU face engine binds to `127.0.0.1:18081` by default and should remain server-local. If you use CompreFace instead, keep its API on a protected server-side network; the browser never receives the CompreFace API key.
- Back up the SQLite database while respecting WAL semantics (stop the service or use SQLite's online backup tooling).
- Treat face data as sensitive biometric data and apply your organization's consent, retention and access-control requirements.
