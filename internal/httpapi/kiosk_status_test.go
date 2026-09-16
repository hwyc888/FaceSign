package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hwyc888/FaceSign/internal/config"
	"github.com/hwyc888/FaceSign/internal/domain"
	"github.com/hwyc888/FaceSign/internal/face"
)

func TestKioskStatusRequiresReachableFaceService(t *testing.T) {
	compreface := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer compreface.Close()

	manager, err := face.NewManager(domain.FaceSettings{
		Provider:           "compreface",
		ServiceURL:         compreface.URL,
		APIKey:             "configured-key",
		Similarity:         0.78,
		DetectionThreshold: 0.80,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	server := &Server{cfg: config.Config{KioskAccessKey: "kiosk-key"}, faces: manager}
	request := httptest.NewRequest(http.MethodGet, "/api/kiosk/status", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, body = %s", response.Code, response.Body.String())
	}
	var status struct {
		FaceEnabled bool   `json:"face_enabled"`
		FaceReady   bool   `json:"face_ready"`
		FaceMessage string `json:"face_message"`
	}
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if !status.FaceEnabled {
		t.Fatal("configured service should report face_enabled=true")
	}
	if status.FaceReady {
		t.Fatal("unreachable service should report face_ready=false")
	}
	if status.FaceMessage == "" {
		t.Fatal("unreachable service should include a Chinese-facing message")
	}
}
