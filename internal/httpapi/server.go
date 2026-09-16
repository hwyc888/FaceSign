package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/hwyc888/FaceSign/internal/attendance"
	"github.com/hwyc888/FaceSign/internal/config"
	"github.com/hwyc888/FaceSign/internal/face"
	"github.com/hwyc888/FaceSign/internal/realtime"
	"github.com/hwyc888/FaceSign/internal/repository"
	"github.com/hwyc888/FaceSign/internal/webui"
)

type Server struct {
	cfg        config.Config
	repo       *repository.Repository
	attendance *attendance.Service
	faces      *face.Manager
	hub        *realtime.Hub
	logger     *slog.Logger
	location   *time.Location
}

func New(cfg config.Config, repo *repository.Repository, attendanceService *attendance.Service, faces *face.Manager, hub *realtime.Hub, logger *slog.Logger, location *time.Location) *Server {
	return &Server{cfg: cfg, repo: repo, attendance: attendanceService, faces: faces, hub: hub, logger: logger, location: location}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/setup/status", s.handleSetupStatus)
	mux.HandleFunc("POST /api/setup", s.handleSetup)
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)
	mux.HandleFunc("GET /api/me", s.requireAuth(s.handleMe))
	mux.HandleFunc("GET /api/kiosk/status", s.handleKioskStatus)
	mux.HandleFunc("POST /api/attendance/recognize", s.handleRecognize)

	mux.HandleFunc("GET /api/events", s.requireAuth(s.handleEvents))
	mux.HandleFunc("GET /api/dashboard/current", s.requireAuth(s.handleDashboard))
	mux.HandleFunc("GET /api/attendance/sessions/{id}/records", s.requireAuth(s.handleSessionRecords))
	mux.HandleFunc("POST /api/attendance/manual", s.requireAuth(s.handleManualAttendance))
	mux.HandleFunc("POST /api/leaves", s.requireAuth(s.handleCreateLeave))
	mux.HandleFunc("GET /api/settings/face", s.requireAdmin(s.handleGetFaceSettings))
	mux.HandleFunc("PUT /api/settings/face", s.requireAdmin(s.handleUpdateFaceSettings))
	mux.HandleFunc("POST /api/settings/face/check", s.requireAdmin(s.handleCheckFaceService))

	mux.HandleFunc("GET /api/users", s.requireAdmin(s.handleListUsers))
	mux.HandleFunc("POST /api/users", s.requireAdmin(s.handleCreateUser))

	mux.HandleFunc("GET /api/classes", s.requireAuth(s.handleListClasses))
	mux.HandleFunc("POST /api/classes", s.requireAdmin(s.handleCreateClass))
	mux.HandleFunc("PUT /api/classes/{id}", s.requireAdmin(s.handleUpdateClass))
	mux.HandleFunc("DELETE /api/classes/{id}", s.requireAdmin(s.handleDeleteClass))

	mux.HandleFunc("GET /api/students", s.requireAuth(s.handleListStudents))
	mux.HandleFunc("POST /api/students", s.requireAdmin(s.handleCreateStudent))
	mux.HandleFunc("PUT /api/students/{id}", s.requireAdmin(s.handleUpdateStudent))
	mux.HandleFunc("DELETE /api/students/{id}", s.requireAdmin(s.handleDeleteStudent))
	mux.HandleFunc("POST /api/students/{id}/face", s.requireAdmin(s.handleEnrollFace))

	mux.HandleFunc("GET /api/courses", s.requireAuth(s.handleListCourses))
	mux.HandleFunc("POST /api/courses", s.requireAdmin(s.handleCreateCourse))
	mux.HandleFunc("PUT /api/courses/{id}", s.requireAdmin(s.handleUpdateCourse))
	mux.HandleFunc("DELETE /api/courses/{id}", s.requireAdmin(s.handleDeleteCourse))
	mux.HandleFunc("GET /api/courses/{id}/students", s.requireAuth(s.handleCourseStudents))
	mux.HandleFunc("POST /api/courses/{id}/students", s.requireAdmin(s.handleAddCourseStudent))
	mux.HandleFunc("DELETE /api/courses/{id}/students/{studentID}", s.requireAdmin(s.handleRemoveCourseStudent))

	mux.HandleFunc("GET /api/schedules", s.requireAuth(s.handleListSchedules))
	mux.HandleFunc("POST /api/schedules", s.requireAdmin(s.handleCreateSchedule))
	mux.HandleFunc("PUT /api/schedules/{id}", s.requireAdmin(s.handleUpdateSchedule))
	mux.HandleFunc("DELETE /api/schedules/{id}", s.requireAdmin(s.handleDeleteSchedule))

	mux.Handle("/", webui.Handler())
	return s.securityHeaders(mux)
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(self), microphone=()")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "face_provider": s.attendance.FaceProvider().Name(), "face_enabled": s.attendance.FaceProvider().Enabled()})
}

func (s *Server) handleKioskStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"face_provider": s.attendance.FaceProvider().Name(), "face_enabled": s.attendance.FaceProvider().Enabled(), "kiosk_key_required": s.cfg.KioskAccessKey != ""})
}
