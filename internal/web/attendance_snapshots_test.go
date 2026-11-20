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
	if err := jpeg.Encode(&data, img, &jpeg.Options{Quality: 80}); err != nil { t.Fatal(err) }
	return data.Bytes()
}

func TestAttendanceSnapshotTimelineAndCacheCleanupAreAdminOnly(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "snapshot-api.db"))
	if err != nil { t.Fatal(err) }
	defer st.Close()
	student, err := st.CreateStudent(ctx, "API-PHOTO-001", "现场截图", "二年1班")
	if err != nil { t.Fatal(err) }
	record, _, err := st.MarkAttendance(ctx, student, .91, time.Now())
	if err != nil { t.Fatal(err) }
	jpegData := makeTestSnapshot(t)
	snapshot, err := st.SaveAttendanceSnapshot(ctx, record.ID, jpegData)
	if err != nil { t.Fatal(err) }
	s := &Server{store: st}

	denied := httptest.NewRequest(http.MethodGet, "/api/attendance/"+strconv.FormatInt(record.ID, 10)+"/snapshots", nil)
	denied = denied.WithContext(context.WithValue(denied.Context(), authContextKey{}, store.User{ID: 22}))
	deniedRec := httptest.NewRecorder()
	s.attendanceSnapshot(deniedRec, denied)
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("non-admin snapshot list status=%d body=%s", deniedRec.Code, deniedRec.Body.String())
	}

	cacheDenied := httptest.NewRequest(http.MethodGet, "/api/cache/attendance-snapshots", nil)
	cacheDenied = cacheDenied.WithContext(context.WithValue(cacheDenied.Context(), authContextKey{}, store.User{ID: 22}))
	cacheDeniedRec := httptest.NewRecorder()
	s.attendanceSnapshotCache(cacheDeniedRec, cacheDenied)
	if cacheDeniedRec.Code != http.StatusForbidden {
		t.Fatalf("non-admin cache status=%d body=%s", cacheDeniedRec.Code, cacheDeniedRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/attendance/"+strconv.FormatInt(record.ID, 10)+"/snapshots", nil)
	listRec := httptest.NewRecorder()
	s.attendanceSnapshot(listRec, withTestAdmin(listReq))
	if listRec.Code != http.StatusOK { t.Fatalf("snapshot timeline status=%d body=%s", listRec.Code, listRec.Body.String()) }
	var timeline []store.AttendanceSnapshot
	if err := json.Unmarshal(listRec.Body.Bytes(), &timeline); err != nil || len(timeline) != 1 || timeline[0].ID != snapshot.ID {
		t.Fatalf("unexpected snapshot timeline: %#v err=%v", timeline, err)
	}

	imageReq := httptest.NewRequest(http.MethodGet, "/api/attendance/snapshots/"+strconv.FormatInt(snapshot.ID, 10), nil)
	imageRec := httptest.NewRecorder()
	s.attendanceSnapshot(imageRec, withTestAdmin(imageReq))
	if imageRec.Code != http.StatusOK || imageRec.Header().Get("Content-Type") != "image/jpeg" || imageRec.Body.Len() == 0 {
		t.Fatalf("snapshot image status=%d headers=%v bytes=%d", imageRec.Code, imageRec.Header(), imageRec.Body.Len())
	}

	cacheGet := httptest.NewRequest(http.MethodGet, "/api/cache/attendance-snapshots", nil)
	cacheRec := httptest.NewRecorder()
	s.attendanceSnapshotCache(cacheRec, withTestAdmin(cacheGet))
	if cacheRec.Code != http.StatusOK { t.Fatalf("cache GET status=%d body=%s", cacheRec.Code, cacheRec.Body.String()) }
	var stats store.AttendanceSnapshotCacheStats
	if err := json.Unmarshal(cacheRec.Body.Bytes(), &stats); err != nil || stats.Count != 1 {
		t.Fatalf("cache stats=%#v err=%v", stats, err)
	}

	cacheDelete := httptest.NewRequest(http.MethodDelete, "/api/cache/attendance-snapshots", nil)
	deleteRec := httptest.NewRecorder()
	s.attendanceSnapshotCache(deleteRec, withTestAdmin(cacheDelete))
	if deleteRec.Code != http.StatusOK { t.Fatalf("cache DELETE status=%d body=%s", deleteRec.Code, deleteRec.Body.String()) }
	if _, err := st.AttendanceSnapshotPath(ctx, snapshot.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("snapshot should be gone after cache cleanup: %v", err)
	}
	records, err := st.ListAttendance(ctx, time.Now().Format("2006-01-02"))
	if err != nil || len(records) != 1 {
		t.Fatalf("cache cleanup must preserve attendance: %#v err=%v", records, err)
	}
}
