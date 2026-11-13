package web

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

func TestManualLowLightAttendanceRequiresAdminAndPendingReview(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "manual-low-light-attendance.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	student, err := st.CreateStudent(ctx, "S-MANUAL", "夜间测试学生", "测试班")
	if err != nil {
		t.Fatal(err)
	}
	settings, err := st.AppSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.LowLightRecognitionMode = true
	if err := st.UpdateAppSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}

	tracker := newRecognitionTracker()
	now := time.Now()
	var decision trackDecision
	for i := 0; i < livenessFastFrames; i++ {
		decision = tracker.Observe("manual-session", []recognitionObservation{{
			Student: student, Similarity: 0.86, LiveScore: 0.007,
			Exposure: 0.25, Sharpness: 0.055, LowLightMode: true,
		}}, now.Add(time.Duration(i)*200*time.Millisecond))[0]
	}
	if !decision.NeedsManualReview || decision.Rejected {
		t.Fatalf("expected pending manual review, got %#v", decision)
	}

	s := &Server{
		store: st, tracker: tracker, logger: slog.Default(), matchThreshold: 0.75,
	}
	body := "{\"track_id\":\"" + decision.TrackID + "\"}"

	nonAdmin := httptest.NewRequest(http.MethodPost, "/api/attendance/manual", strings.NewReader(body))
	nonAdmin.Header.Set("Content-Type", "application/json")
	nonAdmin.Header.Set("X-FaceSign-Session", "manual-session")
	nonAdminRec := httptest.NewRecorder()
	s.manualAttendance(nonAdminRec, nonAdmin)
	if nonAdminRec.Code != http.StatusForbidden {
		t.Fatalf("non-admin manual attendance status=%d body=%s", nonAdminRec.Code, nonAdminRec.Body.String())
	}

	admin := httptest.NewRequest(http.MethodPost, "/api/attendance/manual", strings.NewReader(body))
	admin.Header.Set("Content-Type", "application/json")
	admin.Header.Set("X-FaceSign-Session", "manual-session")
	adminRec := httptest.NewRecorder()
	s.manualAttendance(adminRec, withTestAdmin(admin))
	if adminRec.Code != http.StatusOK {
		t.Fatalf("admin manual attendance status=%d body=%s", adminRec.Code, adminRec.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(adminRec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["manual_confirmed"] != true || result["first_checkin_today"] != true {
		t.Fatalf("unexpected manual attendance response: %#v", result)
	}

	records, err := st.ListAttendance(ctx, time.Now().Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].StudentID != student.ID {
		t.Fatalf("manual confirmation did not create exactly one attendance record: %#v", records)
	}

	repeat := httptest.NewRequest(http.MethodPost, "/api/attendance/manual", strings.NewReader(body))
	repeat.Header.Set("Content-Type", "application/json")
	repeat.Header.Set("X-FaceSign-Session", "manual-session")
	repeatRec := httptest.NewRecorder()
	s.manualAttendance(repeatRec, withTestAdmin(repeat))
	if repeatRec.Code != http.StatusConflict {
		t.Fatalf("repeated manual confirmation status=%d body=%s", repeatRec.Code, repeatRec.Body.String())
	}
	records, err = st.ListAttendance(ctx, time.Now().Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("repeated manual confirmation created duplicate attendance: %#v", records)
	}
}