package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

func TestAutomaticLowLightUncertainFramesDoNotWriteAttendance(t *testing.T) {
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
	for i := 0; i < livenessLowLightMaxFrames; i++ {
		decision = tracker.Observe("auto-session", []recognitionObservation{{
			Student: student, Similarity: 0.86, LiveScore: 0.007,
			Exposure: 0.25, Sharpness: 0.055, LowLightMode: true,
		}}, now.Add(time.Duration(i)*200*time.Millisecond))[0]
	}
	if decision.Verified || decision.Rejected || decision.NeedsAttendance || !decision.TimedOut {
		t.Fatalf("uncertain low-light frames must not trigger automatic attendance: %#v", decision)
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
	if len(records) != 0 {
		t.Fatalf("uncertain low-light frames must not create attendance records: %#v", records)
	}
}

func TestAttendanceSeatsSyncsClassForRecognizedStudent(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "runtime-seat-class-sync.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	student, err := st.CreateStudent(ctx, "SEAT-SYNC-001", "班级同步学生", "自动同步班")
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{store: st}
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/attendance/seats?class_name="+url.QueryEscape(student.ClassName)+"&day="+time.Now().Format("2006-01-02"),
		nil,
	)
	recorder := httptest.NewRecorder()
	server.attendanceSeats(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("attendance seat board status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var board store.AttendanceBoard
	if err := json.Unmarshal(recorder.Body.Bytes(), &board); err != nil {
		t.Fatal(err)
	}
	if board.Class.Name != student.ClassName || board.Total != 1 || board.Students[0].StudentID != student.ID {
		t.Fatalf("recognized student's class seat board was not returned: %#v", board)
	}
}

