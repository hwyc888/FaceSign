# FaceSign Architecture

FaceSign uses a small modular Go architecture. New functionality belongs in its domain module instead of growing a single main file.

## Entry point: cmd/facesign
- main.go: dependency wiring and graceful HTTP lifecycle.
- config.go: command-line configuration and validation.
- platform.go: OS/browser/runtime-library helpers.
- startup_log.go: startup and error log files.

## Face engine: internal/face
- engine.go: YuNet detection and SFace recognition.
- codec.go: embedding serialization.

## Data layer: internal/store
- store.go: SQLite connection lifecycle.
- models.go: domain data structures.
- schema.go: schema creation and migrations.
- classes.go: class CRUD, seat layout and automatic seat arrangement.
- students.go: student operations and seat numbers.
- faces.go: multi-angle face samples.
- attendance.go: attendance records.

SQL stays in the store package; HTTP handlers do not execute SQL directly.

## HTTP layer: internal/web
- server.go: server construction, routes, middleware, static UI.
- status.go: health/version endpoints.
- students.go: student endpoints.
- classes.go: class endpoints.
- enrollment.go: enrollment, duplicate checking, supplemental face samples.
- photo_import.go: ZIP photo analysis, quality gates, duplicate review and batch commit.
- recognition.go: multi-face recognition and check-in.
- attendance.go: attendance records and class seat-board status.
- http_helpers.go: shared request/response helpers.
- embed.go: embedded browser assets.

## Browser UI: internal/web/assets/js
- core.js: shared state/helpers.
- navigation.js: navigation/status.
- camera.js: camera lifecycle/capture.
- recognition.js: multi-face recognition UI.
- enrollment.js: first enrollment and duplicate flow.
- classes.js: class arrangement and seat-layout UI.
- students.js: students, editable class/seat number and face samples.
- seating.js: real-time class seat check-in board.
- photo_import.js: ZIP photo import preview, class editing and confirmation.
- attendance.js: attendance UI.
- boot.js: startup.

## Dependency direction
```text
cmd/facesign
  -> internal/web
  -> internal/store
  -> internal/face

internal/web -> internal/store
internal/web -> internal/face
```

The store and face packages do not depend on the web layer.

## Future features
Add course, timetable, leave, report/export and similar features as separate store/web/UI files. Do not add generic repository/service abstractions until a second implementation actually requires them.
