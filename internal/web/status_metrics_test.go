package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

func TestHealthEndpointIncludesSystemResourcesAndRecognitionRate(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "health-metrics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := &Server{store: st, recognitionMetrics: newRecognitionMetrics()}
	s.recordRecognitionFrame(time.Now(), 4, 3, 3)

	rec := httptest.NewRecorder()
	s.health(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("health status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if _, ok := response["resources"].(map[string]any); !ok {
		t.Fatalf("system resource metrics missing: %#v", response)
	}
	metrics, ok := response["recognition_metrics"].(map[string]any)
	if !ok || metrics["detected_faces"] != float64(4) || metrics["matched_faces"] != float64(3) ||
		metrics["success_rate_percent"] != float64(75) {
		t.Fatalf("unexpected recognition metrics: %#v", response["recognition_metrics"])
	}
}
