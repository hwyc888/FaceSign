package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/hwyc888/FaceSign/internal/domain"
	"github.com/hwyc888/FaceSign/internal/security"
)

type userContextKey struct{}

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	count, err := s.repo.UserCount(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取初始化状态失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"setup_required": count == 0})
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	count, err := s.repo.UserCount(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取初始化状态失败")
		return
	}
	if count != 0 {
		writeError(w, http.StatusConflict, "系统已经完成初始化")
		return
	}
	var request struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	hash, err := security.HashPassword(request.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	user, err := s.repo.CreateUser(r.Context(), request.Username, hash, request.DisplayName, domain.RoleAdmin)
	if err != nil {
		writeError(w, http.StatusConflict, "初始化管理员失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	user, hash, err := s.repo.FindUserByUsername(r.Context(), strings.TrimSpace(request.Username))
	if err != nil || !security.VerifyPassword(hash, request.Password) {
		writeError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	plain, tokenHash, err := security.NewSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "创建登录会话失败")
		return
	}
	expires := time.Now().Add(s.cfg.SessionTTL)
	if err := s.repo.CreateLoginSession(r.Context(), tokenHash, user.ID, expires); err != nil {
		writeError(w, http.StatusInternalServerError, "创建登录会话失败")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "facesign_session", Value: plain, Path: "/", HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(s.cfg.SessionTTL.Seconds())})
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("facesign_session"); err == nil {
		_ = s.repo.DeleteLoginSession(r.Context(), security.HashSessionToken(cookie.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: "facesign_session", Value: "", Path: "/", HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(0, 0)})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, currentUser(r.Context()))
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("facesign_session")
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "请先登录")
			return
		}
		user, err := s.repo.FindUserBySession(r.Context(), security.HashSessionToken(cookie.Value), time.Now())
		if err != nil {
			if err != sql.ErrNoRows {
				s.logger.Warn("session lookup failed", "error", err)
			}
			writeError(w, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, user)))
	}
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r.Context()).Role != domain.RoleAdmin {
			writeError(w, http.StatusForbidden, "需要管理员权限")
			return
		}
		next(w, r)
	})
}

func currentUser(ctx context.Context) domain.User {
	user, _ := ctx.Value(userContextKey{}).(domain.User)
	return user
}
