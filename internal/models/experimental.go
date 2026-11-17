package models

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExperimentalArtifact describes an externally supplied ONNX model.
// Experimental weights are deliberately not bundled until their redistribution
// and deployment licences have been verified.
type ExperimentalArtifact struct {
	Name   string
	SHA256 string
}

// ExperimentalBundle is an optional, separately versioned model set.
// It is not wired into the production inference path yet.
type ExperimentalBundle struct {
	Detector   ExperimentalArtifact
	Recognizer ExperimentalArtifact
	Landmarks2D ExperimentalArtifact
	Landmarks3D ExperimentalArtifact
}

// ValidateExperimentalBundle verifies the presence and integrity of every
// required artifact. Optional landmark models are checked only when named.
// It never downloads weights or changes the active production model set.
func ValidateExperimentalBundle(directory string, bundle ExperimentalBundle) error {
	if strings.TrimSpace(directory) == "" {
		return errors.New("experimental model directory is empty")
	}
	required := []struct {
		label string
		item  ExperimentalArtifact
	}{
		{label: "SCRFD detector", item: bundle.Detector},
		{label: "face recognizer", item: bundle.Recognizer},
	}
	optional := []struct {
		label string
		item  ExperimentalArtifact
	}{
		{label: "2D landmark model", item: bundle.Landmarks2D},
		{label: "3D landmark model", item: bundle.Landmarks3D},
	}
	for _, entry := range required {
		if err := validateExperimentalArtifact(directory, entry.label, entry.item, true); err != nil {
			return err
		}
	}
	for _, entry := range optional {
		if err := validateExperimentalArtifact(directory, entry.label, entry.item, false); err != nil {
			return err
		}
	}
	return nil
}

func validateExperimentalArtifact(directory, label string, artifact ExperimentalArtifact, required bool) error {
	name := strings.TrimSpace(artifact.Name)
	checksum := strings.ToLower(strings.TrimSpace(artifact.SHA256))
	if name == "" && checksum == "" && !required {
		return nil
	}
	if name == "" || checksum == "" {
		return fmt.Errorf("%s requires both filename and SHA-256", label)
	}
	if filepath.Base(name) != name || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("%s has an unsafe model filename %q", label, name)
	}
	if len(checksum) != sha256.Size*2 {
		return fmt.Errorf("%s has an invalid SHA-256", label)
	}
	if _, err := hex.DecodeString(checksum); err != nil {
		return fmt.Errorf("%s has an invalid SHA-256: %w", label, err)
	}
	path := filepath.Join(directory, name)
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("%s model %q: %w", label, name, err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("hash %s model %q: %w", label, name, err)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != checksum {
		return fmt.Errorf("%s model %q checksum mismatch: got %s", label, name, got)
	}
	return nil
}
