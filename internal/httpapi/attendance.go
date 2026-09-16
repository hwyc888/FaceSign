package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hwyc888/FaceSign/internal/domain"
	"github.com/hwyc888/FaceSign/internal/face"
	"github.com/hwyc888/FaceSign/internal/realtime"
	"github.com/hwyc888/FaceSign/internal/repository"
)

func (s *Server) handleRecognize(w http.ResponseWriter, r *http.Request) {
	if s.cfg.KioskAccessKey != "" && r.Header.Get("X-Kiosk-Key") != s.cfg.KioskAccessKey {
		writeError(w, http.StatusUnauthorized, "刷脸终端密钥错误")
		return
	}
	image, err := readMultipartImage(w, r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	classroom := strings.TrimSpace(r.FormValue("classroom"))
	result, err := s.attendance.Recognize(r.Context(), classroom, image, time.Now())
	if err != nil {
		switch {
		case errors.Is(err, face.ErrNoMatch):
			writeError(w, http.StatusNotFound, "未识别到已登记学生")
		case errors.Is(err, face.ErrMultipleFaces):
			writeError(w, http.StatusConflict, "画面中检测到多张人脸，请一次只允许一名学生刷脸")
		case errors.Is(err, face.ErrInvalidAPIKey):
			writeError(w, http.StatusBadGateway, "人脸识别服务 API Key 无效，请联系管理员重新配置")
		case errors.Is(err, face.ErrUnavailable):
			writeError(w, http.StatusServiceUnavailable, "人脸识别服务未启用或当前不可用，请联系管理员检查人脸识别设置")
		case repository.IsNotFound(err):
			writeError(w, http.StatusNotFound, "当前教室没有可签到课程")
		default:
			writeError(w, http.StatusConflict, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	items, err := s.attendance.Dashboard(r.Context(), currentUser(r.Context()), time.Now())
	if err != nil {
		writeError(w, 500, "读取实时考勤失败")
		return
	}
	writeJSON(w, 200, items)
}

func (s *Server) handleSessionRecords(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, "id")
	if !ok {
		return
	}
	if !s.canAccessSession(w, r, id) {
		return
	}
	items, err := s.repo.SessionRecords(r.Context(), id)
	if err != nil {
		writeError(w, 500, "读取考勤明细失败")
		return
	}
	writeJSON(w, 200, items)
}

func (s *Server) handleManualAttendance(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID int64  `json:"session_id"`
		StudentID int64  `json:"student_id"`
		Status    string `json:"status"`
		Note      string `json:"note"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.SessionID <= 0 || req.StudentID <= 0 {
		writeError(w, 400, "课程场次和学生编号不能为空")
		return
	}
	if !s.canAccessSession(w, r, req.SessionID) {
		return
	}
	session, err := s.repo.SessionByID(r.Context(), req.SessionID)
	if err != nil {
		writeError(w, 404, "课程场次不存在")
		return
	}
	enrolled, err := s.repo.StudentEnrolled(r.Context(), session.CourseID, req.StudentID)
	if err != nil {
		writeError(w, 500, "检查学生选课失败")
		return
	}
	if !enrolled {
		writeError(w, 409, "该学生不在此课程名单中")
		return
	}
	record, err := s.repo.RecordManual(r.Context(), req.SessionID, req.StudentID, req.Status, strings.TrimSpace(req.Note), time.Now())
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	s.hub.Publish(realtime.Event{Type: "attendance.updated", Data: map[string]any{"session_id": req.SessionID}})
	writeJSON(w, 200, record)
}

func (s *Server) handleCreateLeave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StudentID  int64  `json:"student_id"`
		ScheduleID int64  `json:"schedule_id"`
		LeaveDate  string `json:"leave_date"`
		Reason     string `json:"reason"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if _, err := time.ParseInLocation("2006-01-02", req.LeaveDate, s.location); err != nil {
		writeError(w, 400, "请假日期格式应为 YYYY-MM-DD")
		return
	}
	user := currentUser(r.Context())
	if user.Role == domain.RoleTeacher {
		ok, err := s.repo.TeacherCanAccessSchedule(r.Context(), user.ID, req.ScheduleID)
		if err != nil {
			writeError(w, 500, "校验课程权限失败")
			return
		}
		if !ok {
			writeError(w, 403, "不能为其他教师课程审批请假")
			return
		}
	}
	item, err := s.repo.CreateLeave(r.Context(), domain.Leave{StudentID: req.StudentID, ScheduleID: req.ScheduleID, LeaveDate: req.LeaveDate, Reason: strings.TrimSpace(req.Reason), ApprovedBy: user.ID})
	if err != nil {
		writeError(w, 409, "登记请假失败: "+err.Error())
		return
	}
	_ = s.attendance.Tick(r.Context(), time.Now())
	s.hub.Publish(realtime.Event{Type: "attendance.updated", Data: map[string]any{"schedule_id": req.ScheduleID}})
	writeJSON(w, 201, item)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "服务器不支持实时推送")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch, cancel := s.hub.Subscribe()
	defer cancel()
	fmt.Fprint(w, "event: ready\ndata: {\"type\":\"ready\"}\n\n")
	flusher.Flush()
	keepAlive := time.NewTicker(20 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case _, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprint(w, "event: refresh\ndata: {\"type\":\"refresh\"}\n\n")
			flusher.Flush()
		case <-keepAlive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) canAccessSession(w http.ResponseWriter, r *http.Request, sessionID int64) bool {
	user := currentUser(r.Context())
	if user.Role == domain.RoleAdmin {
		return true
	}
	ok, err := s.repo.TeacherCanAccessSession(r.Context(), user.ID, sessionID)
	if err != nil {
		writeError(w, 500, "校验课程权限失败")
		return false
	}
	if !ok {
		writeError(w, 403, "无权查看该课程考勤")
		return false
	}
	return true
}
