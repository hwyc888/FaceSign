package web

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

func TestSuccessfulAttendanceSnapshotHelperWritesJPEG(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "snapshot-capture.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	student, err := st.CreateStudent(ctx, "CAPTURE-001", "签到截图", "二年2班")
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := st.MarkAttendance(ctx, student, .9, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 48, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 48; x++ {
			img.Set(x, y, color.RGBA{R: 120, G: 170, B: 220, A: 255})
		}
	}
	s := &Server{store: st, logger: slog.Default()}
	s.saveAttendanceSnapshot(ctx, &record, img)
	if !record.SnapshotAvailable {
		t.Fatal("successful attendance did not receive a snapshot reference")
	}
	snapshots, err := st.ListAttendanceSnapshots(ctx, record.ID)
	if err != nil || len(snapshots) != 1 {
		t.Fatalf("successful attendance snapshot timeline=%#v err=%v", snapshots, err)
	}
	path, err := st.AttendanceSnapshotPath(ctx, snapshots[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoded, err := jpeg.Decode(file)
	if err != nil {
		t.Fatalf("saved attendance snapshot is not a readable JPEG: %v", err)
	}
	if decoded.Bounds().Dx() != 48 || decoded.Bounds().Dy() != 32 {
		t.Fatalf("saved attendance snapshot dimensions=%v, want 48x32", decoded.Bounds())
	}
}
