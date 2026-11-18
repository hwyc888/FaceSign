package models

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLivenessModelSpecSelectsMN3ByDefaultAndMiniFASForABTest(t *testing.T) {
	t.Setenv("FACESIGN_LIVENESS_BACKEND", "")
	spec := livenessModelSpec()
	if spec.Name != "anti-spoof-mn3.onnx" || spec.SHA256 == "" || spec.URL != "" {
		t.Fatalf("MN3 must be the stable default: %#v", spec)
	}
	t.Setenv("FACESIGN_LIVENESS_BACKEND", "minifas")
	spec = livenessModelSpec()
	if spec.Name != "minifasnet-v2se.onnx" || spec.SHA256 != "af2381b88f38769222ed93379e12444e2a50814575de1c46170de570c55a42b6" || spec.URL == "" {
		t.Fatalf("MiniFAS A/B selection failed: %#v", spec)
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
