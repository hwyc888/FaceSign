package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHomePageHasNoRedirect(t *testing.T) {
	home, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{home: home, version: "test-version"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	s.root(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / returned %d, want %d", rec.Code, http.StatusOK)
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("GET / unexpectedly redirected to %q", location)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("GET / Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}
	if rec.Header().Get("X-FaceSign-Version") != "test-version" {
		t.Fatalf("GET / X-FaceSign-Version = %q", rec.Header().Get("X-FaceSign-Version"))
	}
	if !strings.Contains(rec.Body.String(), "<title>FaceSign</title>") {
		t.Fatal("GET / did not serve FaceSign index.html")
	}
}
