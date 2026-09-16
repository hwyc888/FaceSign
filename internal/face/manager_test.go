package face

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hwyc888/FaceSign/internal/domain"
)

func TestManagerStatusChecksCompreFaceAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/recognition/subjects/" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("x-api-key") != "good-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"subjects":[]}`))
	}))
	defer server.Close()

	manager, err := NewManager(domain.FaceSettings{
		Provider:           "compreface",
		ServiceURL:         server.URL,
		APIKey:             "good-key",
		Similarity:         0.78,
		DetectionThreshold: 0.80,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	status := manager.Status(context.Background())
	if !status.Reachable || !status.Enabled || !status.Configured {
		t.Fatalf("status = %#v, want ready service", status)
	}
}

func TestManagerStatusReportsDisabledInChinese(t *testing.T) {
	manager, err := NewManager(domain.FaceSettings{
		Provider:           "disabled",
		ServiceURL:         "http://127.0.0.1:8000",
		Similarity:         0.78,
		DetectionThreshold: 0.80,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	status := manager.Status(context.Background())
	if status.Enabled || status.Reachable || status.Message != "人脸识别服务尚未启用" {
		t.Fatalf("unexpected disabled status: %#v", status)
	}
}
