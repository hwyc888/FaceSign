package models

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
