package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/hwyc888/FaceSign/internal/domain"
	"github.com/hwyc888/FaceSign/internal/security"
)

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	items, err := s.repo.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取用户失败")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	hash, err := security.HashPassword(request.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	user, err := s.repo.CreateUser(r.Context(), request.Username, hash, request.DisplayName, request.Role)
	if err != nil {
		writeError(w, http.StatusConflict, "创建用户失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (s *Server) handleListClasses(w http.ResponseWriter, r *http.Request) {
	items, err := s.repo.ListClasses(r.Context())
	if err != nil {
		writeError(w, 500, "读取班级失败")
		return
	}
	writeJSON(w, 200, items)
}
func (s *Server) handleCreateClass(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	item, err := s.repo.CreateClass(r.Context(), req.Name)
	if err != nil {
		writeError(w, 409, "创建班级失败: "+err.Error())
		return
	}
	writeJSON(w, 201, item)
}
func (s *Server) handleUpdateClass(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.repo.UpdateClass(r.Context(), id, req.Name); err != nil {
		writeError(w, 409, "修改班级失败: "+err.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) handleDeleteClass(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.repo.DeleteClass(r.Context(), id); err != nil {
		writeError(w, 409, "删除班级失败，该班级可能仍有关联学生")
		return
	}
	w.WriteHeader(204)
}

func (s *Server) handleListStudents(w http.ResponseWriter, r *http.Request) {
	items, err := s.repo.ListStudents(r.Context())
	if err != nil {
		writeError(w, 500, "读取学生失败")
		return
	}
	writeJSON(w, 200, items)
}
func (s *Server) handleCreateStudent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StudentNo string `json:"student_no"`
		Name      string `json:"name"`
		ClassID   int64  `json:"class_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	item, err := s.repo.CreateStudent(r.Context(), req.StudentNo, req.Name, req.ClassID)
	if err != nil {
		writeError(w, 409, "创建学生失败: "+err.Error())
		return
	}
	writeJSON(w, 201, item)
}
func (s *Server) handleUpdateStudent(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		StudentNo string `json:"student_no"`
		Name      string `json:"name"`
		ClassID   int64  `json:"class_id"`
		Active    bool   `json:"active"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.repo.UpdateStudent(r.Context(), id, req.StudentNo, req.Name, req.ClassID, req.Active); err != nil {
		writeError(w, 409, "修改学生失败: "+err.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) handleDeleteStudent(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.repo.DeleteStudent(r.Context(), id); err != nil {
		writeError(w, 409, "删除学生失败: "+err.Error())
		return
	}
	w.WriteHeader(204)
}

func (s *Server) handleEnrollFace(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, "id")
	if !ok {
		return
	}
	image, err := readMultipartImage(w, r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	imageID, err := s.attendance.EnrollFace(r.Context(), id, image)
	if err != nil {
		writeError(w, 502, "人脸样本录入失败: "+err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"image_id": imageID, "student_id": id})
}

func (s *Server) handleListCourses(w http.ResponseWriter, r *http.Request) {
	items, err := s.repo.ListCourses(r.Context())
	if err != nil {
		writeError(w, 500, "读取课程失败")
		return
	}
	writeJSON(w, 200, items)
}
func (s *Server) handleCreateCourse(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string `json:"name"`
		TeacherID int64  `json:"teacher_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	item, err := s.repo.CreateCourse(r.Context(), req.Name, req.TeacherID)
	if err != nil {
		writeError(w, 409, "创建课程失败: "+err.Error())
		return
	}
	writeJSON(w, 201, item)
}
func (s *Server) handleUpdateCourse(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Name      string `json:"name"`
		TeacherID int64  `json:"teacher_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.repo.UpdateCourse(r.Context(), id, req.Name, req.TeacherID); err != nil {
		writeError(w, 409, "修改课程失败: "+err.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) handleDeleteCourse(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.repo.DeleteCourse(r.Context(), id); err != nil {
		writeError(w, 409, "删除课程失败: "+err.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) handleCourseStudents(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, "id")
	if !ok {
		return
	}
	items, err := s.repo.CourseStudents(r.Context(), id)
	if err != nil {
		writeError(w, 500, "读取课程学生失败")
		return
	}
	writeJSON(w, 200, items)
}
func (s *Server) handleAddCourseStudent(w http.ResponseWriter, r *http.Request) {
	courseID, ok := parsePathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		StudentID int64 `json:"student_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.repo.AddStudentToCourse(r.Context(), courseID, req.StudentID); err != nil {
		writeError(w, 409, "添加学生失败: "+err.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) handleRemoveCourseStudent(w http.ResponseWriter, r *http.Request) {
	courseID, ok := parsePathID(w, r, "id")
	if !ok {
		return
	}
	studentID, ok := parsePathID(w, r, "studentID")
	if !ok {
		return
	}
	if err := s.repo.RemoveStudentFromCourse(r.Context(), courseID, studentID); err != nil {
		writeError(w, 409, "移除学生失败: "+err.Error())
		return
	}
	w.WriteHeader(204)
}

func (s *Server) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	items, err := s.repo.ListSchedules(r.Context())
	if err != nil {
		writeError(w, 500, "读取课表失败")
		return
	}
	writeJSON(w, 200, items)
}
func (s *Server) handleCreateSchedule(w http.ResponseWriter, r *http.Request) {
	var item domain.Schedule
	if !decodeJSON(w, r, &item) {
		return
	}
	created, err := s.repo.CreateSchedule(r.Context(), item)
	if err != nil {
		writeError(w, 409, "创建课表失败: "+err.Error())
		return
	}
	writeJSON(w, 201, created)
}
func (s *Server) handleUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, "id")
	if !ok {
		return
	}
	var item domain.Schedule
	if !decodeJSON(w, r, &item) {
		return
	}
	item.ID = id
	if err := s.repo.UpdateSchedule(r.Context(), item); err != nil {
		writeError(w, 409, "修改课表失败: "+err.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) handleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.repo.DeleteSchedule(r.Context(), id); err != nil {
		writeError(w, 409, "删除课表失败: "+err.Error())
		return
	}
	w.WriteHeader(204)
}

func boolFromForm(value string) bool {
	parsed, _ := strconv.ParseBool(strings.TrimSpace(value))
	return parsed
}
