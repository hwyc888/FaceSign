package web

import (
	"errors"
	"net/http"
	"time"
)

func (s *Server) attendance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	items, err := s.store.ListAttendance(r.Context(), r.URL.Query().Get("day"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) attendanceSeats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	board, err := s.store.AttendanceSeatBoard(
		r.Context(),
		r.URL.Query().Get("class_name"),
		r.URL.Query().Get("day"),
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, board)
}


type manualAttendanceRequest struct {
	TrackID string `json:"track_id"`
}

func (s *Server) manualAttendance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !requireAdmin(w, r) {
		return
	}
	var in manualAttendanceRequest
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if in.TrackID == "" {
		writeError(w, http.StatusBadRequest, errors.New("缺少待人工核验的人脸 Track"))
		return
	}

	s.manualAttendanceMu.Lock()
	defer s.manualAttendanceMu.Unlock()

	lowLightMode, err := s.store.LowLightRecognitionMode(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !lowLightMode {
		writeError(w, http.StatusConflict, errors.New("夜间低光模式已关闭，请重新识别"))
		return
	}

	sessionID := recognitionSessionID(r)
	candidate, similarity, ok := s.tracker.ManualReviewCandidate(sessionID, in.TrackID, time.Now())
	if !ok {
		writeError(w, http.StatusConflict, errors.New("人工核验已过期或该人脸不符合人工核验条件，请重新识别"))
		return
	}
	if similarity < s.matchThreshold {
		writeError(w, http.StatusConflict, errors.New("人脸匹配可信度不足，请重新识别"))
		return
	}
	student, err := s.store.StudentByID(r.Context(), candidate.ID)
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("学生资料已不存在，请重新识别"))
		return
	}
	record, created, err := s.store.MarkAttendance(r.Context(), student, similarity, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.tracker.CommitManual(sessionID, in.TrackID)
	if _, err := s.store.RecordRecognitionStats(r.Context(), []int64{student.ID}, nil); err != nil {
		s.logger.Warn("record manual attendance recognition stats failed", "student_id", student.ID, "error", err)
	}
	s.logger.Info("manual low-light attendance confirmed",
		"student_id", student.ID,
		"student_no", student.StudentNo,
		"operator", authUser(r).Username,
	)
	writeJSON(w, http.StatusOK, map[string]any{
		"student": student,
		"attendance": record,
		"first_checkin_today": created,
		"manual_confirmed": true,
	})
}
