package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/hwyc888/FaceSign/internal/store"
)

func (s *Server) students(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.store.ListStudents(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, items)
	case http.MethodPost:
		var in struct {
			StudentNo string `json:"student_no"`
			Name      string `json:"name"`
			ClassName string `json:"class_name"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		student, err := s.store.CreateStudent(r.Context(), in.StudentNo, in.Name, in.ClassName)
		if err != nil {
			writeError(w, http.StatusBadRequest, friendlyStudentError(err))
			return
		}
		writeJSON(w, http.StatusCreated, student)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) studentAction(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/students/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, errors.New("student not found"))
		return
	}
	studentID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || studentID <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("invalid student id"))
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodDelete:
			if err := s.store.DeleteStudent(r.Context(), studentID); err != nil {
				writeError(w, http.StatusNotFound, err)
				return
			}
			s.refreshFaceCacheAfterMutation(r.Context())
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		case http.MethodPut:
			var in struct {
				StudentNo *string `json:"student_no"`
				Name      *string `json:"name"`
				ClassName *string `json:"class_name"`
				SeatNo    *int    `json:"seat_no"`
			}
			if err := decodeJSON(r, &in); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if in.StudentNo == nil && in.Name == nil && in.ClassName == nil && in.SeatNo == nil {
				writeError(w, http.StatusBadRequest, errors.New("没有需要修改的学生资料"))
				return
			}

			var updated *store.Student
			if in.StudentNo != nil || in.Name != nil {
				current, err := s.store.StudentByID(r.Context(), studentID)
				if err != nil {
					writeError(w, http.StatusNotFound, err)
					return
				}
				studentNo := current.StudentNo
				name := current.Name
				className := current.ClassName
				if in.StudentNo != nil {
					studentNo = strings.TrimSpace(*in.StudentNo)
				}
				if in.Name != nil {
					name = strings.TrimSpace(*in.Name)
				}
				if in.ClassName != nil {
					className = strings.TrimSpace(*in.ClassName)
				}
				exists, err := s.store.ClassExists(r.Context(), className)
				if err != nil {
					writeError(w, http.StatusInternalServerError, err)
					return
				}
				if !exists {
					writeError(w, http.StatusBadRequest, errors.New("请选择设置中已经建立的班级"))
					return
				}
				student, err := s.store.UpdateStudentProfile(r.Context(), studentID, studentNo, name, className)
				if err != nil {
					writeError(w, http.StatusBadRequest, friendlyStudentError(err))
					return
				}
				updated = &student
			} else if in.ClassName != nil {
				className := strings.TrimSpace(*in.ClassName)
				exists, err := s.store.ClassExists(r.Context(), className)
				if err != nil {
					writeError(w, http.StatusInternalServerError, err)
					return
				}
				if !exists {
					writeError(w, http.StatusBadRequest, errors.New("请选择设置中已经建立的班级"))
					return
				}
				if err := s.store.UpdateStudentClass(r.Context(), studentID, className); err != nil {
					writeError(w, http.StatusBadRequest, err)
					return
				}
			}

			if in.SeatNo != nil {
				if err := s.store.UpdateStudentSeat(r.Context(), studentID, *in.SeatNo); err != nil {
					writeError(w, http.StatusBadRequest, err)
					return
				}
			}

			s.refreshFaceCacheAfterMutation(r.Context())
			response := map[string]any{"ok": true}
			if updated != nil {
				if in.SeatNo != nil {
					refreshed, err := s.store.StudentByID(r.Context(), studentID)
					if err == nil {
						*updated = refreshed
					}
				}
				response["student"] = *updated
			}
			writeJSON(w, http.StatusOK, response)
		default:
			methodNotAllowed(w)
		}
		return
	}

	if len(parts) == 2 && parts[1] == "face" && r.Method == http.MethodPost {
		s.addFaceSample(w, r, studentID)
		return
	}
	if len(parts) == 2 && parts[1] == "faces" {
		switch r.Method {
		case http.MethodGet:
			items, err := s.store.ListStudentFaceSamples(r.Context(), studentID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, items)
		case http.MethodPost:
			s.addFaceSample(w, r, studentID)
		default:
			methodNotAllowed(w)
		}
		return
	}
	if len(parts) == 3 && parts[1] == "faces" && r.Method == http.MethodDelete {
		sampleID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || sampleID <= 0 {
			writeError(w, http.StatusBadRequest, errors.New("invalid face sample id"))
			return
		}
		if err := s.store.DeleteFaceSample(r.Context(), studentID, sampleID); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		s.refreshFaceCacheAfterMutation(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	methodNotAllowed(w)
}

func friendlyStudentError(err error) error {
	if strings.Contains(err.Error(), "students.student_no") {
		return errors.New("该学号已经存在，请检查后再录入")
	}
	return err
}

