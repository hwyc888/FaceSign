package web

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/hwyc888/FaceSign/internal/store"
	"golang.org/x/crypto/bcrypt"
)

func (s *Server) users(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := s.store.ListUsers(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, items)
	case http.MethodPost:
		var in struct {
			Username         string `json:"username"`
			Password         string `json:"password"`
			CanEditStudents  bool   `json:"can_edit_students"`
			CanAddStudents   bool   `json:"can_add_students"`
			CanResampleFaces bool   `json:"can_resample_faces"`
			CanArrangeSeats  bool   `json:"can_arrange_seats"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		in.Username = strings.TrimSpace(in.Username)
		if err := validateUsername(in.Username); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := validatePassword(in.Password); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		user, err := s.store.CreateUser(r.Context(), in.Username, string(hash), store.User{
			CanEditStudents: in.CanEditStudents,
			CanAddStudents: in.CanAddStudents,
			CanResampleFaces: in.CanResampleFaces,
			CanArrangeSeats: in.CanArrangeSeats,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, user)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) userAction(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/users/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusBadRequest, errors.New("用户编号不正确"))
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 || len(parts) > 2 || (len(parts) == 2 && parts[1] != "clone") {
		writeError(w, http.StatusBadRequest, errors.New("用户编号不正确"))
		return
	}
	if len(parts) == 2 && parts[1] == "clone" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var in struct {
			Usernames []string `json:"usernames"`
			Password  string   `json:"password"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if len(in.Usernames) == 0 || len(in.Usernames) > 100 {
			writeError(w, http.StatusBadRequest, errors.New("一次只能复制 1 到 100 个账号"))
			return
		}
		if err := validatePassword(in.Password); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		seen := make(map[string]struct{}, len(in.Usernames))
		credentials := make([]store.UserCloneCredential, 0, len(in.Usernames))
		for _, rawUsername := range in.Usernames {
			username := strings.TrimSpace(rawUsername)
			if err := validateUsername(username); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			key := strings.ToLower(username)
			if _, exists := seen[key]; exists {
				writeError(w, http.StatusBadRequest, fmt.Errorf("批量生成的用户名重复: %s", username))
				return
			}
			seen[key] = struct{}{}
			hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			credentials = append(credentials, store.UserCloneCredential{
				Username: username, PasswordHash: string(hash),
			})
		}
		created, err := s.store.CloneUsers(r.Context(), id, credentials)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, sql.ErrNoRows) {
				status = http.StatusNotFound
			}
			writeError(w, status, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"ok": true, "created": len(created), "users": created,
		})
		return
	}
	switch r.Method {
	case http.MethodPut:
		var in struct {
			CanEditStudents  *bool  `json:"can_edit_students"`
			CanAddStudents   *bool  `json:"can_add_students"`
			CanResampleFaces *bool  `json:"can_resample_faces"`
			CanArrangeSeats  *bool  `json:"can_arrange_seats"`
			Password         string `json:"password"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		users, err := s.store.ListUsers(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		var user store.User
		found := false
		for _, item := range users {
			if item.ID == id {
				user = item
				found = true
				break
			}
		}
		if !found {
			writeError(w, http.StatusNotFound, errors.New("用户不存在"))
			return
		}
		if user.IsAdmin {
			writeError(w, http.StatusForbidden, errors.New("管理员账号权限不可修改"))
			return
		}
		if in.CanEditStudents != nil { user.CanEditStudents = *in.CanEditStudents }
		if in.CanAddStudents != nil { user.CanAddStudents = *in.CanAddStudents }
		if in.CanResampleFaces != nil { user.CanResampleFaces = *in.CanResampleFaces }
		if in.CanArrangeSeats != nil { user.CanArrangeSeats = *in.CanArrangeSeats }
		passwordHash := ""
		if strings.TrimSpace(in.Password) != "" {
			if err := validatePassword(in.Password); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			passwordHash = string(hash)
		}
		if err := s.store.UpdateUserAccess(r.Context(), id, user, passwordHash); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case http.MethodDelete:
		if err := s.store.DeleteUser(r.Context(), id); err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, sql.ErrNoRows) {
				status = http.StatusNotFound
			}
			writeError(w, status, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		methodNotAllowed(w)
	}
}
