package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const PinnedCommit = "de5287c66e9e37e9f804686bf63f5a0974f68f72"

const defaultBaseURL = "https://raw.githubusercontent.com/hwyc888/FaceSign/" + PinnedCommit + "/models"

type Paths struct {
	Detector   string
	Recognizer string
	Liveness   string
	Directory  string
}

type spec struct {
	Name   string
	SHA256 string
}

var pinned = []spec{
	{Name: "face_detection_yunet_2023mar.onnx", SHA256: "8f2383e4dd3cfbb4553ea8718107fc0423210dc964f9f4280604804ed2552fa4"},
	{Name: "face_recognition_sface_2021dec.onnx", SHA256: "0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79"},
	{Name: "anti-spoof-mn3.onnx", SHA256: "c4c99af04603b62d7e44f6f4daeb33e0daeccc696008c0b1d62f6f5cebbb3262"},
}

func Resolve(assetsPath string) (Paths, error) {
	return resolve(assetsPath, candidateDirectories(assetsPath), defaultBaseURL, pinned)
}

func candidateDirectories(assetsPath string) []string {
	var dirs []string
	if custom := strings.TrimSpace(os.Getenv("FACESIGN_MODEL_DIR")); custom != "" {
		dirs = append(dirs, custom)
	}
	dirs = append(dirs, filepath.Join(assetsPath, "models"))
	if runtime.GOOS == "windows" {
		if programData := strings.TrimSpace(os.Getenv("ProgramData")); programData != "" {
			dirs = append(dirs, filepath.Join(programData, "FaceSign", "models"))
		}
	}
	if cacheDir, err := os.UserCacheDir(); err == nil && cacheDir != "" {
		dirs = append(dirs, filepath.Join(cacheDir, "FaceSign", "models"))
	}
	return uniqueCleanPaths(dirs)
}

func resolve(assetsPath string, candidates []string, baseURL string, specs []spec) (Paths, error) {
	_ = assetsPath
	if len(candidates) == 0 {
		return Paths{}, fmt.Errorf("no model storage directory is available")
	}

	for _, dir := range candidates {
		if allValid(dir, specs) {
			return buildPaths(dir), nil
		}
	}

	target, err := chooseTarget(candidates, specs)
	if err != nil {
		return Paths{}, err
	}

	for _, model := range specs {
		targetPath := filepath.Join(target, model.Name)
		if validModel(targetPath, model.SHA256) {
			continue
		}

		copied := false
		for _, dir := range candidates {
			source := filepath.Join(dir, model.Name)
			if dir == target || !validModel(source, model.SHA256) {
				continue
			}
			if err := copyVerified(source, targetPath, model.SHA256); err != nil {
				return Paths{}, fmt.Errorf("reuse model %s: %w", model.Name, err)
			}
			copied = true
			break
		}
		if copied {
			continue
		}

		if err := downloadVerified(baseURL+"/"+model.Name, targetPath, model.SHA256); err != nil {
			return Paths{}, fmt.Errorf("download model %s: %w", model.Name, err)
		}
	}
	return buildPaths(target), nil
}

func chooseTarget(candidates []string, specs []spec) (string, error) {
	type scored struct {
		dir   string
		count int
		order int
	}
	var writable []scored
	for i, dir := range candidates {
		if !canWriteDirectory(dir) {
			continue
		}
		count := 0
		for _, model := range specs {
			if validModel(filepath.Join(dir, model.Name), model.SHA256) {
				count++
			}
		}
		writable = append(writable, scored{dir: dir, count: count, order: i})
	}
	if len(writable) == 0 {
		return "", fmt.Errorf("no writable model directory is available; checked %s", strings.Join(candidates, ", "))
	}
	sort.SliceStable(writable, func(i, j int) bool {
		if writable[i].count != writable[j].count {
			return writable[i].count > writable[j].count
		}
		return writable[i].order < writable[j].order
	})
	return writable[0].dir, nil
}

func canWriteDirectory(dir string) bool {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	f, err := os.CreateTemp(dir, ".facesign-write-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

func allValid(dir string, specs []spec) bool {
	for _, model := range specs {
		if !validModel(filepath.Join(dir, model.Name), model.SHA256) {
			return false
		}
	}
	return true
}

func validModel(path, expectedSHA string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return false
	}
	return hex.EncodeToString(hash.Sum(nil)) == strings.ToLower(expectedSHA)
}

func copyVerified(source, target, expectedSHA string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	return writeVerified(target, expectedSHA, func(out io.Writer) error {
		_, err := io.Copy(out, in)
		return err
	})
}

func downloadVerified(url, target, expectedSHA string) error {
	client := &http.Client{Timeout: 90 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", response.Status)
	}
	return writeVerified(target, expectedSHA, func(out io.Writer) error {
		_, err := io.Copy(out, response.Body)
		return err
	})
}

func writeVerified(target, expectedSHA string, write func(io.Writer) error) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(target), filepath.Base(target)+".download-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	hash := sha256.New()
	if err := write(io.MultiWriter(temp, hash)); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != strings.ToLower(expectedSHA) {
		return fmt.Errorf("SHA-256 mismatch: got %s", actual)
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tempName, target)
}

func buildPaths(dir string) Paths {
	return Paths{
		Detector:   filepath.Join(dir, "face_detection_yunet_2023mar.onnx"),
		Recognizer: filepath.Join(dir, "face_recognition_sface_2021dec.onnx"),
		Liveness:   filepath.Join(dir, "anti-spoof-mn3.onnx"),
		Directory:  dir,
	}
}

func uniqueCleanPaths(paths []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "." || path == "" {
			continue
		}
		key := strings.ToLower(path)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, path)
	}
	return out
}
