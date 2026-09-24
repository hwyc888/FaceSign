package web

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hwyc888/FaceSign/internal/store"
)

func TestAppSettingsAPI(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "settings-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := &Server{store: st, logger: slog.Default()}

	get := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	getRec := httptest.NewRecorder()
	s.settings(getRec, get)
	if getRec.Code != http.StatusOK {
		t.Fatalf("default GET status=%d body=%s", getRec.Code, getRec.Body.String())
	}
	var current store.AppSettings
	if err := json.Unmarshal(getRec.Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if current.AutoStartCheckin {
		t.Fatal("auto start check-in should default off")
	}
	if !current.RealtimeStatusEnabled {
		t.Fatal("realtime status should default on")
	}

	put := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"auto_start_checkin":true}`))
	put.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	s.settings(putRec, put)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", putRec.Code, putRec.Body.String())
	}

	persisted, err := st.AppSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !persisted.AutoStartCheckin {
		t.Fatal("PUT did not persist auto start check-in")
	}
	if !persisted.RealtimeStatusEnabled {
		t.Fatal("partial auto-start update should preserve realtime status")
	}

	statusPut := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"realtime_status_enabled":false}`))
	statusPut.Header.Set("Content-Type", "application/json")
	statusRec := httptest.NewRecorder()
	s.settings(statusRec, statusPut)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("realtime status PUT status=%d body=%s", statusRec.Code, statusRec.Body.String())
	}

	persisted, err = st.AppSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if persisted.RealtimeStatusEnabled {
		t.Fatal("PUT did not disable realtime status")
	}
	if !persisted.AutoStartCheckin {
		t.Fatal("partial realtime status update should preserve auto start check-in")
	}
}
