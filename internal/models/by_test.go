package models

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLivenessModelSpecSelectsSilentFaceByDefaultAndLegacyBackends(t *testing.T) {
	t.Setenv("FACESIGN_LIVENESS_BACKEND", "")
	spec := livenessModelSpec()
	if spec.Name != "MiniFASNetV2.onnx" || spec.SHA256 != "b32929adc2d9c34b9486f8c4c7bc97c1b69bc0ea9befefc380e4faae4e463907" || spec.URL == "" {
		t.Fatalf("Silent-Face ensemble must be the default: %#v", spec)
	}
	t.Setenv("FACESIGN_LIVENESS_BACKEND", "mn3")
	spec = livenessModelSpec()
	if spec.Name != "anti-spoof-mn3.onnx" || spec.SHA256 == "" || spec.URL != "" {
		t.Fatalf("MN3 legacy selection failed: %#v", spec)
	}
	t.Setenv("FACESIGN_LIVENESS_BACKEND", "minifas")
	spec = livenessModelSpec()
	if spec.Name != "minifasnet-v2se.onnx" || spec.SHA256 != "af2381b88f38769222ed93379e12444e2a50814575de1c46170de570c55a42b6" || spec.URL == "" {
		t.Fatalf("MiniFAS A/B selection failed: %#v", spec)
	}

	t.Setenv("FACESIGN_LIVENESS_BACKEND", "unknown-value")
	if backend := livenessModelBackend(); backend != "silent" {
		t.Fatalf("unknown backend=%q want silent", backend)
	}
	spec = livenessModelSpec()
	if spec.Name != "MiniFASNetV2.onnx" {
		t.Fatalf("unknown backend must resolve to the current Silent-Face default: %#v", spec)
	}
}

func TestReadByChecksums(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SHA256SUMS")
	content := strings.Repeat("a", 64) + "  scrfd_10g.onnx\n" + strings.Repeat("b", 64) + "  arcface_w600k_r50.onnx\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readByChecksums(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["scrfd_10g.onnx"] != strings.Repeat("a", 64) || got["arcface_w600k_r50.onnx"] != strings.Repeat("b", 64) {
		t.Fatalf("unexpected manifest entries: %#v", got)
	}
}

func TestReadByChecksumsRejectsMissingManifest(t *testing.T) {
	if _, err := readByChecksums(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected missing checksum manifest error")
	}
}
