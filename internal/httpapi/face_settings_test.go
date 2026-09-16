package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hwyc888/FaceSign/internal/domain"
)

func TestFaceSettingsResponseDoesNotExposeAPIKey(t *testing.T) {
	response := faceSettingsForResponse(domain.FaceSettings{
		Provider:           "compreface",
		ServiceURL:         "http://127.0.0.1:8000",
		APIKey:             "top-secret-key",
		Similarity:         0.78,
		DetectionThreshold: 0.80,
	})
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if strings.Contains(string(raw), "top-secret-key") {
		t.Fatalf("API key leaked in response: %s", raw)
	}
	if !response.APIKeyConfigured {
		t.Fatal("expected api_key_configured=true")
	}
}
