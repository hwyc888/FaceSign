# FaceSign architecture

FaceSign is intentionally split by responsibility. The executable entry point contains no business logic.

## Layers

- `cmd/facesign`: process entry point only.
- `internal/app`: application wiring, HTTP server lifecycle, TLS and graceful shutdown.
- `internal/config`: environment / `facesign.env` configuration.
- `internal/database`: SQLite connection, WAL settings and schema migrations.
- `internal/domain`: shared business entities and attendance status constants.
- `internal/repository`: SQL persistence for users, academic data and attendance data.
- `internal/security`: Argon2id password hashes and opaque server-side sessions.
- `internal/face`: face-recognition provider interface, runtime provider manager, local CPU REST adapter and optional CompreFace adapter.
- `internal/faceengine`: native CPU inference service using ONNX Runtime, YuNet/SFace and SQLite face embeddings.
- `internal/attendance`: schedule-driven attendance rules and background finalization.
- `internal/realtime`: in-process publish/subscribe hub for teacher dashboard refresh.
- `internal/httpapi`: HTTP API, authorization and request validation.
- `internal/webui`: embedded responsive browser UI and classroom kiosk.
- `deploy`: Windows Service and Linux systemd deployment helpers.

## Attendance state machine

1. A weekly schedule defines course, classroom, start/end time, grace minutes and how early check-in opens.
2. The scheduler creates the day's attendance session automatically once the check-in window opens.
3. A kiosk submits a captured image and classroom name.
4. The face provider returns a subject and similarity score.
5. FaceSign resolves the subject to a student and verifies that the student is enrolled in the active course.
6. `recognized_at <= class_start + grace` is `on_time`; later recognition before class end is `late`.
7. Approved leave records are materialized as `leave` for the session.
8. At course end, every still-missing active student is written as `absent` and the session is closed.
9. `(session_id, student_id)` is unique, so repeated captures update one record instead of creating duplicates.

## Concurrency model

Go's HTTP server handles requests concurrently. SQLite runs in WAL mode with a 5-second busy timeout, short write transactions and uniqueness constraints. The connection pool allows concurrent readers while SQLite serializes the brief writes. This is suitable for a school LAN deployment with multiple teacher dashboards and multiple classroom kiosks. If write volume later exceeds one SQLite server's design envelope, the repository boundary allows migration to PostgreSQL without changing the attendance or HTTP layers.

## Face recognition boundary

FaceSign does not fake recognition inside the attendance process. `internal/face.Provider` defines enrollment, health-check and recognition operations. The recommended production provider is `localcpu`: the separately supervised `facesign-face-engine` Go executable, which dynamically loads the CPU-only ONNX Runtime library shipped in the same application directory and runs YuNet + SFace models locally. It listens on `127.0.0.1:18081`, stores embeddings in SQLite `faces.db`, and requires no Python, Docker, CUDA or GPU. Keeping inference in a separate native service isolates model/runtime faults from the attendance HTTP server while preserving a portable application bundle. `compreface` remains available for schools that already run an external CompreFace service, while `disabled` lets the management platform start before a face engine is configured. Provider settings are persisted in SQLite and can be changed at runtime without restarting FaceSign.

## Data protection

Passwords are stored with Argon2id. Browser sessions use random opaque tokens; only SHA-256 token digests are stored in SQLite. Attendance history is protected by restrictive foreign keys so deleting a schedule or student with existing attendance history is rejected rather than silently erasing records.
