package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAttendanceSnapshotsKeepFullDayTimelineAndClearWithoutDeletingAttendance(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "snapshots.db"))
	if err != nil { t.Fatal(err) }
	defer st.Close()

	student, err := st.CreateStudent(ctx, "PHOTO-001", "截图测试", "二年1班")
	if err != nil { t.Fatal(err) }
	record, _, err := st.MarkAttendance(ctx, student, .88, time.Date(2026, 11, 20, 8, 0, 0, 0, time.Local))
	if err != nil { t.Fatal(err) }

	first, err := st.SaveAttendanceSnapshot(ctx, record.ID, []byte("morning-photo"))
	if err != nil { t.Fatal(err) }
	second, err := st.SaveAttendanceSnapshot(ctx, record.ID, []byte("afternoon-photo"))
	if err != nil { t.Fatal(err) }
	if first.ID == second.ID { t.Fatal("each recognition event must receive its own snapshot id") }

	items, err := st.ListAttendanceSnapshots(ctx, record.ID)
	if err != nil || len(items) != 2 {
		t.Fatalf("expected two snapshots in daily timeline: %#v err=%v", items, err)
	}
	path, err := st.AttendanceSnapshotPath(ctx, first.ID)
	if err != nil { t.Fatal(err) }
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "morning-photo" {
		t.Fatalf("first snapshot content=%q err=%v", data, err)
	}

	attendance, err := st.ListAttendance(ctx, "2026-11-20")
	if err != nil || len(attendance) != 1 || !attendance[0].SnapshotAvailable || attendance[0].SnapshotCount != 2 {
		t.Fatalf("attendance must report all daily snapshots: %#v err=%v", attendance, err)
	}
	stats, err := st.AttendanceSnapshotCacheStats(ctx)
	if err != nil || stats.Count != 2 || stats.Bytes != int64(len("morning-photo")+len("afternoon-photo")) {
		t.Fatalf("snapshot cache stats=%#v err=%v", stats, err)
	}

	removed, err := st.ClearAttendanceSnapshots(ctx)
	if err != nil || removed.Count != 2 {
		t.Fatalf("clear snapshots removed=%#v err=%v", removed, err)
	}
	if _, err := st.AttendanceSnapshotPath(ctx, first.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("snapshot should be unavailable after cleanup: %v", err)
	}
	attendance, err = st.ListAttendance(ctx, "2026-11-20")
	if err != nil || len(attendance) != 1 || attendance[0].SnapshotAvailable || attendance[0].SnapshotCount != 0 {
		t.Fatalf("cleanup must preserve attendance but clear snapshot timeline: %#v err=%v", attendance, err)
	}
}

func TestLegacySingleAttendanceSnapshotMigratesIntoTimeline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-snapshots.db")
	db, err := sql.Open("sqlite", path)
	if err != nil { t.Fatal(err) }
	_, err = db.Exec(`
		CREATE TABLE students(id INTEGER PRIMARY KEY,student_no TEXT NOT NULL UNIQUE,name TEXT NOT NULL,class_name TEXT NOT NULL DEFAULT '',seat_no INTEGER NOT NULL DEFAULT 0,created_at INTEGER NOT NULL);
		CREATE TABLE attendance(id INTEGER PRIMARY KEY,student_id INTEGER NOT NULL,day TEXT NOT NULL,checked_at INTEGER NOT NULL,last_seen_at INTEGER NOT NULL DEFAULT 0,recognition_count INTEGER NOT NULL DEFAULT 1,similarity REAL NOT NULL,UNIQUE(student_id,day));
		INSERT INTO students VALUES(1,'LEGACY-001','旧照片学生','二年1班',0,1);
		INSERT INTO attendance VALUES(1,1,'2026-11-20',100,100,1,.9);
		CREATE TABLE attendance_snapshots(attendance_id INTEGER PRIMARY KEY,file_name TEXT NOT NULL,created_at INTEGER NOT NULL);
		INSERT INTO attendance_snapshots VALUES(1,'attendance-1.jpg',200);
	`)
	if err != nil { db.Close(); t.Fatal(err) }
	if err := db.Close(); err != nil { t.Fatal(err) }

	st, err := Open(path)
	if err != nil { t.Fatal(err) }
	defer st.Close()
	items, err := st.ListAttendanceSnapshots(context.Background(), 1)
	if err != nil || len(items) != 1 || items[0].RecognizedAt != time.Unix(200, 0).Local().Format("2006-01-02 15:04:05.000") {
		t.Fatalf("legacy snapshot timestamp was not preserved: %#v err=%v", items, err)
	}
	var count int
	if err := st.db.QueryRow("SELECT COUNT(*) FROM attendance_snapshots").Scan(&count); err != nil || count != 1 {
		t.Fatalf("legacy snapshot count=%d err=%v", count, err)
	}
}
