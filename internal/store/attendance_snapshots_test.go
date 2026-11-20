package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAttendanceSnapshotsListAndClearWithoutDeletingAttendance(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "snapshots.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	student, err := st.CreateStudent(ctx, "PHOTO-001", "截图测试", "二年1班")
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := st.MarkAttendance(ctx, student, .88, time.Date(2026, 11, 20, 8, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	jpeg := []byte("test-jpeg-data")
	available, err := st.SaveAttendanceSnapshot(ctx, record.ID, jpeg)
	if err != nil || !available {
		t.Fatalf("save snapshot available=%v err=%v", available, err)
	}
	path, err := st.AttendanceSnapshotPath(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(jpeg) {
		t.Fatalf("snapshot content=%q err=%v", data, err)
	}

	items, err := st.ListAttendance(ctx, "2026-11-20")
	if err != nil || len(items) != 1 || !items[0].SnapshotAvailable {
		t.Fatalf("attendance snapshot not listed: %#v err=%v", items, err)
	}
	stats, err := st.AttendanceSnapshotCacheStats(ctx)
	if err != nil || stats.Count != 1 || stats.Bytes != int64(len(jpeg)) {
		t.Fatalf("snapshot cache stats=%#v err=%v", stats, err)
	}

	removed, err := st.ClearAttendanceSnapshots(ctx)
	if err != nil || removed.Count != 1 || removed.Bytes != int64(len(jpeg)) {
		t.Fatalf("clear snapshots removed=%#v err=%v", removed, err)
	}
	if _, err := st.AttendanceSnapshotPath(ctx, record.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("snapshot should be unavailable after cleanup: %v", err)
	}
	items, err = st.ListAttendance(ctx, "2026-11-20")
	if err != nil || len(items) != 1 || items[0].SnapshotAvailable {
		t.Fatalf("cleanup must preserve attendance but clear snapshot flag: %#v err=%v", items, err)
	}
}

func TestAttendanceSnapshotSaveIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "snapshot-idempotent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	student, err := st.CreateStudent(ctx, "PHOTO-002", "重复截图", "二年1班")
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := st.MarkAttendance(ctx, student, .9, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveAttendanceSnapshot(ctx, record.ID, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveAttendanceSnapshot(ctx, record.ID, []byte("second")); err != nil {
		t.Fatal(err)
	}
	path, err := st.AttendanceSnapshotPath(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "first" {
		t.Fatalf("repeated recognition replaced the first snapshot: %q err=%v", data, err)
	}
}
