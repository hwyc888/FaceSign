package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

func TestAutomaticLowLightAttendanceWritesOnce(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "auto-low-light-attendance.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	student, err := st.CreateStudent(ctx, "S-AUTO", "夜间测试学生", "测试班")
	if err != nil {
		t.Fatal(err)
	}

	tracker := newRecognitionTracker()
	now := time.Now()
	var decision trackDecision
	for i := 0; i < livenessMinFrames; i++ {
		decision = tracker.Observe("auto-session", []recognitionObservation{{
			Student: student, Similarity: 0.86, LiveScore: 0.007,
			Exposure: 0.25, Sharpness: 0.055, LowLightMode: true,
		}}, now.Add(time.Duration(i)*200*time.Millisecond))[0]
	}
	if !decision.Verified || decision.Rejected || !decision.NeedsAttendance {
		t.Fatalf("expected automatic low-light attendance decision: %#v", decision)
	}

	_, created, err := st.MarkAttendance(ctx, student, decision.Similarity, now)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected first automatic check-in to be created")
	}
	tracker.Commit("auto-session", decision.TrackID)

	repeated := tracker.Observe("auto-session", []recognitionObservation{{
		Student: student, Similarity: 0.87, LiveScore: 0.01,
		Exposure: 0.25, Sharpness: 0.055, LowLightMode: true,
	}}, now.Add(time.Second))[0]
	if !repeated.Verified || repeated.NeedsAttendance {
		t.Fatalf("automatic attendance must not be written twice: %#v", repeated)
	}

	s := &Server{store: st}
	req := httptest.NewRequest(http.MethodGet, "/api/attendance?day="+now.Format("2006-01-02"), nil)
	rec := httptest.NewRecorder()
	s.attendance(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("attendance status=%d body=%s", rec.Code, rec.Body.String())
	}
	var records []store.Attendance
	if err := json.Unmarshal(rec.Body.Bytes(), &records); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].StudentID != student.ID {
		t.Fatalf("automatic check-in should leave one attendance record: %#v", records)
	}
}
