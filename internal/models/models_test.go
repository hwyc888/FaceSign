package models

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveReusesExistingModels(t *testing.T) {
	assets := t.TempDir()
	cache := t.TempDir()
	specs := []spec{
		fakeSpec("one.onnx", []byte("one")),
		fakeSpec("two.onnx", []byte("two")),
		fakeSpec("three.onnx", []byte("three")),
	}
	for _, model := range specs {
		if err := os.WriteFile(filepath.Join(cache, model.Name), fakeData(model.Name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := resolve(assets, []string{filepath.Join(assets, "models"), cache}, "http://127.0.0.1:1", specs)
	if err != nil {
		t.Fatal(err)
	}
	if paths.Directory != cache {
		t.Fatalf("expected existing model cache %q, got %q", cache, paths.Directory)
	}
}

func TestResolveDownloadsOnlyMissingModels(t *testing.T) {
	assets := t.TempDir()
	target := filepath.Join(assets, "models")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	specs := []spec{
		fakeSpec("one.onnx", []byte("one")),
		fakeSpec("two.onnx", []byte("two")),
		fakeSpec("three.onnx", []byte("three")),
	}
	if err := os.WriteFile(filepath.Join(target, "one.onnx"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch filepath.Base(r.URL.Path) {
		case "two.onnx":
			_, _ = w.Write([]byte("two"))
		case "three.onnx":
			_, _ = w.Write([]byte("three"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	paths, err := resolve(assets, []string{target}, server.URL, specs)
	if err != nil {
		t.Fatal(err)
	}
	if paths.Directory != target {
		t.Fatalf("unexpected target: %q", paths.Directory)
	}
	if requests != 2 {
		t.Fatalf("downloaded %d models, want 2", requests)
	}
	if !allValid(target, specs) {
		t.Fatal("resolved models are not valid")
	}
}

func TestResolveRejectsBadDownload(t *testing.T) {
	assets := t.TempDir()
	target := filepath.Join(assets, "models")
	specs := []spec{fakeSpec("one.onnx", []byte("expected"))}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("wrong"))
	}))
	defer server.Close()

	if _, err := resolve(assets, []string{target}, server.URL, specs); err == nil {
		t.Fatal("expected checksum failure")
	}
}

func fakeSpec(name string, content []byte) spec {
	sum := sha256.Sum256(content)
	return spec{Name: name, SHA256: hex.EncodeToString(sum[:])}
}

func fakeData(name string) []byte {
	switch name {
	case "one.onnx":
		return []byte("one")
	case "two.onnx":
		return []byte("two")
	case "three.onnx":
		return []byte("three")
	default:
		return nil
	}
}
