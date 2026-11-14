package web

import (
	"net/url"
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

func TestMarkAttendanceWithoutLivenessWritesOnlyOncePerSession(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "disable-liveness-attendance.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	student, err := st.CreateStudent(ctx, "S-NO-LIVENESS", "关闭活体测试", "测试班")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{store: st, tracker: newRecognitionTracker()}
	req := httptest.NewRequest(http.MethodPost, "/api/recognize", nil)
	now := time.Now()

	first, created, err := s.markAttendanceWithoutLiveness(req, "disabled-session", student, 0.92, now)
	if err != nil {
		t.Fatal(err)
	}
	if first == nil || !created {
		t.Fatalf("first direct check-in was not recorded: record=%#v created=%v", first, created)
	}
	committed := s.tracker.Observe("disabled-session", []recognitionObservation{{
		Student: student, Similarity: 0.93, LiveScore: 0.9, Exposure: 1, Sharpness: 1,
	}}, now.Add(100*time.Millisecond))[0]
	if !committed.Verified || committed.NeedsAttendance || committed.LivenessStatus != "活体检测已关闭" {
		t.Fatalf("direct check-in track should retain disabled-liveness status: %#v", committed)
	}

	second, created, err := s.markAttendanceWithoutLiveness(req, "disabled-session", student, 0.93, now.Add(200*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if second != nil || created {
		t.Fatalf("same recognition session should not write attendance twice: record=%#v created=%v", second, created)
	}

	third, created, err := s.markAttendanceWithoutLiveness(req, "disabled-session", student, 0.94, now.Add(livenessTrackTTL+time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if third == nil || created {
		t.Fatalf("expired recognition track should resume normal attendance updates: record=%#v created=%v", third, created)
	}

	records, err := st.ListAttendance(ctx, now.Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].StudentID != student.ID || records[0].RecognitionCount != 2 {
		t.Fatalf("direct check-in should deduplicate active sessions and resume after track expiry: %#v", records)
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

