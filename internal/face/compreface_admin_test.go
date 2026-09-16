package face

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestProvisionCompreFaceCreatesApplicationAndRecognitionService(t *testing.T) {
	var applicationCreated atomic.Bool
	var modelCreated atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin/user/register":
			if r.Method != http.MethodPost {
				t.Fatalf("register method = %s", r.Method)
			}
			w.WriteHeader(http.StatusCreated)
		case "/admin/oauth/token":
			if r.Header.Get("Authorization") != comprefaceBasicAuthorization {
				t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("username") != "facesign@local.invalid" || r.Form.Get("password") != "secret-password" {
				t.Fatalf("unexpected login form: %v", r.Form)
			}
			writeTestJSON(t, w, map[string]string{"access_token": "admin-token"})
		case "/admin/apps":
			assertBearer(t, r)
			writeTestJSON(t, w, []any{})
		case "/admin/app":
			assertBearer(t, r)
			applicationCreated.Store(true)
			w.WriteHeader(http.StatusCreated)
			writeTestJSON(t, w, map[string]string{"id": "app-id", "name": defaultCompreFaceApplication})
		case "/admin/app/app-id/models":
			assertBearer(t, r)
			writeTestJSON(t, w, []any{})
		case "/admin/app/app-id/model":
			assertBearer(t, r)
			modelCreated.Store(true)
			w.WriteHeader(http.StatusCreated)
			writeTestJSON(t, w, map[string]string{
				"id": "model-id", "name": defaultCompreFaceService, "type": "RECOGNITION", "apiKey": "recognition-key",
			})
		case "/api/v1/recognition/subjects/":
			if r.Header.Get("x-api-key") != "recognition-key" {
				t.Fatalf("x-api-key = %q", r.Header.Get("x-api-key"))
			}
			writeTestJSON(t, w, map[string]any{"subjects": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := ProvisionCompreFace(context.Background(), CompreFaceProvisionOptions{
		BaseURL:  server.URL,
		Email:    "facesign@local.invalid",
		Password: "secret-password",
	})
	if err != nil {
		t.Fatalf("ProvisionCompreFace() error = %v", err)
	}
	if result.APIKey != "recognition-key" {
		t.Fatalf("API key = %q", result.APIKey)
	}
	if !applicationCreated.Load() || !modelCreated.Load() {
		t.Fatal("application and recognition service were not created")
	}
}

func TestProvisionCompreFaceReusesExistingObjectsAfterRegistrationConflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin/user/register":
			http.Error(w, `{"message":"already registered"}`, http.StatusBadRequest)
		case "/admin/oauth/token":
			writeTestJSON(t, w, map[string]string{"access_token": "admin-token"})
		case "/admin/apps":
			writeTestJSON(t, w, []comprefaceApplication{{ID: "existing-app", Name: defaultCompreFaceApplication}})
		case "/admin/app/existing-app/models":
			writeTestJSON(t, w, []comprefaceModel{{
				ID: "existing-model", Name: defaultCompreFaceService, Type: "RECOGNITION", APIKey: "existing-key",
			}})
		case "/api/v1/recognition/subjects/":
			writeTestJSON(t, w, map[string]any{"subjects": []any{}})
		case "/admin/app", "/admin/app/existing-app/model":
			t.Fatalf("unexpected create request: %s", r.URL.Path)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := ProvisionCompreFace(context.Background(), CompreFaceProvisionOptions{
		BaseURL:  server.URL,
		Email:    "facesign@local.invalid",
		Password: "secret-password",
	})
	if err != nil {
		t.Fatalf("ProvisionCompreFace() error = %v", err)
	}
	if result.APIKey != "existing-key" {
		t.Fatalf("API key = %q", result.APIKey)
	}
}

func assertBearer(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Header.Get("Authorization") != "Bearer admin-token" {
		t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
	}
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}
