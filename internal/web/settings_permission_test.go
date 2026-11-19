package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hwyc888/FaceSign/internal/store"
)

func requestAsUser(req *http.Request, user store.User) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), authContextKey{}, user))
}

func TestSettingsPermissionCanEditGeneralSettingsButCannotDisableLiveness(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "settings-permission.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	user, err := st.CreateUser(context.Background(), "settings-operator", "hash", store.User{CanEditSettings: true})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{store: st}

	req := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"auto_start_checkin":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.settings(rec, requestAsUser(req, user))
	if rec.Code != http.StatusOK {
		t.Fatalf("settings permission update status=%d body=%s", rec.Code, rec.Body.String())
	}
	settings, err := st.AppSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !settings.AutoStartCheckin {
		t.Fatal("settings permission did not persist the allowed setting")
	}

	req = httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"disable_liveness_detection":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	s.settings(rec, requestAsUser(req, user))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin liveness change status=%d body=%s, want 403", rec.Code, rec.Body.String())
	}
}

func TestSettingsPermissionCanCreateClassConfiguration(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "class-settings-permission.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	user, err := st.CreateUser(context.Background(), "class-operator", "hash", store.User{CanEditSettings: true})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{store: st}
	req := httptest.NewRequest(http.MethodPost, "/api/classes", strings.NewReader(`{"name":"高一一班","seat_rows":6,"seats_per_row":8}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.classes(rec, requestAsUser(req, user))
	if rec.Code != http.StatusCreated {
		t.Fatalf("class settings permission status=%d body=%s", rec.Code, rec.Body.String())
	}
}
