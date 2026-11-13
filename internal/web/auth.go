package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/hwyc888/FaceSign/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const (
	authCookieName = "facesign_session"
	authSessionTTL = 12 * time.Hour
	authReleaseGrace = 7 * 24 * time.Hour
)

type authContextKey struct{}

func authUser(r *http.Request) store.User {
	user, _ := r.Context().Value(authContextKey{}).(store.User)
	return user
}

func sessionTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newSessionToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, sessionTokenHash(token), nil
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: authCookieName, Value: token, Path: "/", Expires: expires,
		MaxAge: int(time.Until(expires).Seconds()), HttpOnly: true,
		Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: authCookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode,
	})
}

func requestSessionToken(r *http.Request) string {
	cookie, err := r.Cookie(authCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, user store.User) error {
	token, hash, err := newSessionToken()
	if err != nil {
		return err
	}
	expires := time.Now().Add(authSessionTTL)
	if err := s.store.CreateAuthSession(r.Context(), hash, user.ID, expires); err != nil {
		return err
	}
	// Keep the opaque cookie briefly after server-side expiry so the client can
	// still release an active camera stream; all other APIs reject expired sessions.
	setSessionCookie(w, r, token, expires.Add(authReleaseGrace))
	return nil
}

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	count, err := s.store.UserCount(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if count == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"setup_required": true, "authenticated": false})
		return
	}
	token := requestSessionToken(r)
	if token == "" {
		writeJSON(w, http.StatusOK, map[string]any{"setup_required": false, "authenticated": false})
		return
	}
	user, err := s.store.UserBySession(r.Context(), sessionTokenHash(token), time.Now())
	if err != nil {
		clearSessionCookie(w, r)
		writeJSON(w, http.StatusOK, map[string]any{"setup_required": false, "authenticated": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"setup_required": false, "authenticated": true, "user": user})
}

func (s *Server) authSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !requestIsLoopback(r) {
		writeError(w, http.StatusForbidden, errors.New("首次创建管理员必须在运行 FaceSign 的电脑本机浏览器完成"))
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
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
	user, err := s.store.CreateInitialAdmin(r.Context(), in.Username, string(hash))
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "已经初始化") {
			status = http.StatusConflict
		}
		writeError(w, status, err)
		return
	}
	if err := s.issueSession(w, r, user); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "user": user})
}

func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil && !requestIsLoopback(r) {
		writeError(w, http.StatusForbidden, errors.New("远程登录必须使用 HTTPS 地址"))
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	user, err := s.store.UserByUsername(r.Context(), strings.TrimSpace(in.Username))
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password)) != nil {
		writeError(w, http.StatusUnauthorized, errors.New("用户名或密码不正确"))
		return
	}
	if err := s.issueSession(w, r, user); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": user})
}

func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if token := requestSessionToken(r); token != "" {
		if err := s.store.DeleteAuthSession(r.Context(), sessionTokenHash(token)); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) authPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	user := authUser(r)
	var in struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.CurrentPassword)) != nil {
		writeError(w, http.StatusBadRequest, errors.New("当前密码不正确"))
		return
	}
	if err := validatePassword(in.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.store.UpdateUserPassword(r.Context(), user.ID, string(hash)); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.store.DeleteOtherAuthSessions(r.Context(), user.ID, sessionTokenHash(requestSessionToken(r))); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func validateUsername(username string) error {
	if len(username) < 3 || len(username) > 32 {
		return errors.New("用户名长度必须为 3 到 32 个字符")
	}
	for _, r := range username {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' && r != '.' {
			return errors.New("用户名只能包含字母、数字、下划线、短横线和点")
		}
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < 10 {
		return errors.New("密码至少需要 10 个字符")
	}
	if len(password) > 72 {
		return errors.New("密码不能超过 72 个字节")
	}
	return nil
}

func requestIsLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func isPublicAPIPath(path string) bool {
	switch path {
	case "/api/auth/status", "/api/auth/setup", "/api/auth/login", "/api/auth/logout",
		"/api/health", "/api/version", "/api/camera-agents/frame":
		return true
	default:
		return false
	}
}

func isCameraReleaseRequest(r *http.Request) bool {
	return r.Method == http.MethodPost &&
		strings.HasPrefix(r.URL.Path, "/api/cameras/") &&
		strings.HasSuffix(r.URL.Path, "/release")
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") || isPublicAPIPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if r.TLS == nil && !requestIsLoopback(r) {
			writeError(w, http.StatusForbidden, errors.New("远程访问必须使用 HTTPS 地址"))
			return
		}
		token := requestSessionToken(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, errors.New("请先登录 FaceSign"))
			return
		}
		tokenHash := sessionTokenHash(token)
		user, err := s.store.UserBySession(r.Context(), tokenHash, time.Now())
		if err != nil {
			// Keep an expired session cookie long enough for the browser to release any
			// active camera capture; it is not accepted for any other API operation.
			expiredUser, expiredErr := s.store.UserBySessionForRelease(r.Context(), tokenHash)
			if expiredErr == nil {
				if isCameraReleaseRequest(r) {
					user = expiredUser
					err = nil
				} else {
					writeError(w, http.StatusUnauthorized, errors.New("登录已过期，请重新登录"))
					return
				}
			} else {
				clearSessionCookie(w, r)
				writeError(w, http.StatusUnauthorized, errors.New("登录已过期，请重新登录"))
				return
			}
		}
		ctx := context.WithValue(r.Context(), authContextKey{}, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if authUser(r).IsAdmin {
		return true
	}
	writeError(w, http.StatusForbidden, errors.New("只有管理员可以执行此操作"))
	return false
}

func requirePermission(w http.ResponseWriter, r *http.Request, allowed bool, message string) bool {
	if authUser(r).IsAdmin || allowed {
		return true
	}
	writeError(w, http.StatusForbidden, errors.New(message))
	return false
}
