package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

func makeTestSnapshot(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: 80, G: 140, B: 210, A: 255})
		}
	}
	var data bytes.Buffer
	if err := jpeg.Encode(&data, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestAttendanceSnapshotAPIAndCacheCleanupAreAdminOnly(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "snapshot-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	student, err := st.CreateStudent(ctx, "API-PHOTO-001", "现场截图", "二年1班")
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := st.MarkAttendance(ctx, student, .91, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	jpegData := makeTestSnapshot(t)
	if _, err := st.SaveAttendanceSnapshot(ctx, record.ID, jpegData); err != nil {
		t.Fatal(err)
	}
	s := &Server{store: st}

	denied := httptest.NewRequest(http.MethodGet, "/api/attendance/"+strconv.FormatInt(record.ID, 10)+"/snapshot", nil)
	denied = denied.WithContext(context.WithValue(denied.Context(), authContextKey{}, store.User{ID: 22}))
	deniedRec := httptest.NewRecorder()
	s.attendanceSnapshot(deniedRec, denied)
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("non-admin snapshot status=%d body=%s", deniedRec.Code, deniedRec.Body.String())
	}

	cacheDenied := httptest.NewRequest(http.MethodGet, "/api/cache/attendance-snapshots", nil)
	cacheDenied = cacheDenied.WithContext(context.WithValue(cacheDenied.Context(), authContextKey{}, store.User{ID: 22}))
	cacheDeniedRec := httptest.NewRecorder()
	s.attendanceSnapshotCache(cacheDeniedRec, cacheDenied)
	if cacheDeniedRec.Code != http.StatusForbidden {
		t.Fatalf("non-admin cache status=%d body=%s", cacheDeniedRec.Code, cacheDeniedRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/attendance/"+strconv.FormatInt(record.ID, 10)+"/snapshot", nil)
	rec := httptest.NewRecorder()
	s.attendanceSnapshot(rec, withTestAdmin(req))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/jpeg" || rec.Body.Len() == 0 {
		t.Fatalf("snapshot response status=%d headers=%v bytes=%d", rec.Code, rec.Header(), rec.Body.Len())
	}

	cacheGet := httptest.NewRequest(http.MethodGet, "/api/cache/attendance-snapshots", nil)
	cacheRec := httptest.NewRecorder()
	s.attendanceSnapshotCache(cacheRec, withTestAdmin(cacheGet))
	if cacheRec.Code != http.StatusOK {
		t.Fatalf("cache GET status=%d body=%s", cacheRec.Code, cacheRec.Body.String())
	}
	var stats store.AttendanceSnapshotCacheStats
	if err := json.Unmarshal(cacheRec.Body.Bytes(), &stats); err != nil || stats.Count != 1 {
		t.Fatalf("cache stats=%#v err=%v", stats, err)
	}

	cacheDelete := httptest.NewRequest(http.MethodDelete, "/api/cache/attendance-snapshots", nil)
	deleteRec := httptest.NewRecorder()
	s.attendanceSnapshotCache(deleteRec, withTestAdmin(cacheDelete))
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("cache DELETE status=%d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
	if _, err := st.AttendanceSnapshotPath(ctx, record.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("snapshot should be gone after cache cleanup: %v", err)
	}
	records, err := st.ListAttendance(ctx, time.Now().Format("2006-01-02"))
	if err != nil || len(records) != 1 {
		t.Fatalf("cache cleanup must preserve attendance: %#v err=%v", records, err)
	}
}
