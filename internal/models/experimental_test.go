package models

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeExperimentalFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func TestValidateExperimentalBundle(t *testing.T) {
	dir := t.TempDir()
	detectorHash := writeExperimentalFixture(t, dir, "scrfd.onnx", "detector")
	recognizerHash := writeExperimentalFixture(t, dir, "resnet50.onnx", "recognizer")
	landmarkHash := writeExperimentalFixture(t, dir, "landmark106.onnx", "landmarks")

	bundle := ExperimentalBundle{
		Detector: ExperimentalArtifact{Name: "scrfd.onnx", SHA256: detectorHash},
		Recognizer: ExperimentalArtifact{Name: "resnet50.onnx", SHA256: recognizerHash},
		Landmarks2D: ExperimentalArtifact{Name: "landmark106.onnx", SHA256: landmarkHash},
	}
	if err := ValidateExperimentalBundle(dir, bundle); err != nil {
		t.Fatalf("valid bundle rejected: %v", err)
	}
}

func TestValidateExperimentalBundleAllowsOmittedOptionalModels(t *testing.T) {
	dir := t.TempDir()
	detectorHash := writeExperimentalFixture(t, dir, "scrfd.onnx", "detector")
	recognizerHash := writeExperimentalFixture(t, dir, "resnet50.onnx", "recognizer")
	bundle := ExperimentalBundle{
		Detector: ExperimentalArtifact{Name: "scrfd.onnx", SHA256: detectorHash},
		Recognizer: ExperimentalArtifact{Name: "resnet50.onnx", SHA256: recognizerHash},
	}
	if err := ValidateExperimentalBundle(dir, bundle); err != nil {
		t.Fatalf("optional landmark models should be optional: %v", err)
	}
}

func TestValidateExperimentalBundleRejectsChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	writeExperimentalFixture(t, dir, "scrfd.onnx", "detector")
	recognizerHash := writeExperimentalFixture(t, dir, "resnet50.onnx", "recognizer")
	bundle := ExperimentalBundle{
		Detector: ExperimentalArtifact{Name: "scrfd.onnx", SHA256: strings.Repeat("0", 64)},
		Recognizer: ExperimentalArtifact{Name: "resnet50.onnx", SHA256: recognizerHash},
	}
	if err := ValidateExperimentalBundle(dir, bundle); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func TestValidateExperimentalBundleRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	recognizerHash := writeExperimentalFixture(t, dir, "resnet50.onnx", "recognizer")
	bundle := ExperimentalBundle{
		Detector: ExperimentalArtifact{Name: "../scrfd.onnx", SHA256: strings.Repeat("0", 64)},
		Recognizer: ExperimentalArtifact{Name: "resnet50.onnx", SHA256: recognizerHash},
	}
	if err := ValidateExperimentalBundle(dir, bundle); err == nil || !strings.Contains(err.Error(), "unsafe model filename") {
		t.Fatalf("expected unsafe filename error, got %v", err)
	}
}
