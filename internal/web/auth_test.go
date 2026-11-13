package web

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

func TestAdminBootstrapIsLocalOnlyAndCreatesLoginSession(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "auth-bootstrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := &Server{store: st, logger: slog.Default()}

	body := strings.NewReader(\`{"username":"school-admin","password":"secure-pass-123"}\`)
	remote := httptest.NewRequest(http.MethodPost, "/api/auth/setup", body)
	remote.RemoteAddr = "192.0.2.20:12345"
	remoteRec := httptest.NewRecorder()
	s.authSetup(remoteRec, remote)
	if remoteRec.Code != http.StatusForbidden {
		t.Fatalf("remote setup status=%d body=%s", remoteRec.Code, remoteRec.Body.String())
	}

	body = strings.NewReader(\`{"username":"school-admin","password":"secure-pass-123"}\`)
	local := httptest.NewRequest(http.MethodPost, "/api/auth/setup", body)
	local.RemoteAddr = "127.0.0.1:12345"
	localRec := httptest.NewRecorder()
	s.authSetup(localRec, local)
	if localRec.Code != http.StatusCreated {
		t.Fatalf("local setup status=%d body=%s", localRec.Code, localRec.Body.String())
	}
	cookies := localRec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != authCookieName || !cookies[0].HttpOnly {
		t.Fatalf("administrator session cookie missing or insecure: %#v", cookies)
	}

	status := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	status.RemoteAddr = "127.0.0.1:12345"
	status.AddCookie(cookies[0])
	statusRec := httptest.NewRecorder()
	s.authStatus(statusRec, status)
	var response struct {
		Authenticated bool       `json:"authenticated"`
		User          store.User `json:"user"`
	}
	if err := json.Unmarshal(statusRec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Authenticated || !response.User.IsAdmin || response.User.Username != "school-admin" {
		t.Fatalf("unexpected auth status: %#v", response)
	}
}

func TestAuthenticatedRemoteAPIsRequireHTTPS(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "auth-https.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	admin, err := st.CreateInitialAdmin(context.Background(), "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	token, tokenHash, err := newSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAuthSession(context.Background(), tokenHash, admin.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	s := &Server{store: st}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authUser(r).IsAdmin {
			t.Error("authenticated administrator was not attached to request")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	handler := s.authMiddleware(next)

	httpRequest := httptest.NewRequest(http.MethodGet, "/api/students", nil)
	httpRequest.RemoteAddr = "192.0.2.20:12345"
	httpRequest.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	httpRec := httptest.NewRecorder()
	handler.ServeHTTP(httpRec, httpRequest)
	if httpRec.Code != http.StatusForbidden {
		t.Fatalf("remote HTTP status=%d body=%s", httpRec.Code, httpRec.Body.String())
	}

	httpsRequest := httptest.NewRequest(http.MethodGet, "/api/students", nil)
	httpsRequest.RemoteAddr = "192.0.2.20:12345"
	httpsRequest.TLS = &tls.ConnectionState{}
	httpsRequest.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	httpsRec := httptest.NewRecorder()
	handler.ServeHTTP(httpsRec, httpsRequest)
	if httpsRec.Code != http.StatusNoContent {
		t.Fatalf("remote HTTPS status=%d body=%s", httpsRec.Code, httpsRec.Body.String())
	}
}

func TestStudentProfileUpdateRequiresPermission(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "student-permissions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	class, err := st.CreateClassWithLayout(context.Background(), "一班", 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	student, err := st.CreateStudent(context.Background(), "S001", "原姓名", class.Name)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{store: st, logger: slog.Default()}

	body := strings.NewReader(\`{"name":"新姓名"}\`)
	req := httptest.NewRequest(http.MethodPut, "/api/students/1", body)
	req = req.WithContext(context.WithValue(req.Context(), authContextKey{}, store.User{}))
	rec := httptest.NewRecorder()
	s.studentAction(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unprivileged update status=%d body=%s", rec.Code, rec.Body.String())
	}
	current, err := st.StudentByID(context.Background(), student.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Name != "原姓名" {
		t.Fatalf("unauthorized request changed student name to %q", current.Name)
	}

	body = strings.NewReader(\`{"student_no":"S002","name":"新姓名","class_name":"一班"}\`)
	req = httptest.NewRequest(http.MethodPut, "/api/students/1", body)
	req = req.WithContext(context.WithValue(req.Context(), authContextKey{}, store.User{CanEditStudents:true}))
	rec = httptest.NewRecorder()
	s.studentAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("permitted update status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSeatAssignmentRequiresSeparatePermission(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "seat-permissions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	class, err := st.CreateClassWithLayout(context.Background(), "一班", 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	student, err := st.CreateStudent(context.Background(), "S001", "学生", class.Name)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{store: st, logger: slog.Default()}
	body := strings.NewReader(\`{"seat_no":2}\`)
	req := httptest.NewRequest(http.MethodPut, "/api/students/1", body)
	req = req.WithContext(context.WithValue(req.Context(), authContextKey{}, store.User{CanEditStudents:true}))
	rec := httptest.NewRecorder()
	s.studentAction(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("seat update without seat permission status=%d body=%s", rec.Code, rec.Body.String())
	}
	current, err := st.StudentByID(context.Background(), student.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.SeatNo != 0 {
		t.Fatalf("unauthorized request changed seat to %d", current.SeatNo)
	}
}
