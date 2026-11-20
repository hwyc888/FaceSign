package models

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	bySCRFDFile = "scrfd_10g.onnx"
	byArcFaceFile = "arcface_w600k_r50.onnx"
	byLandmark2DFile = "2d106det.onnx"
	byLandmark3DFile = "1k3d68.onnx"
)

// ResolveBy resolves the By model set. SCRFD/ArcFace weights are never fetched
// automatically because upstream InsightFace model-zoo weights are restricted
// to non-commercial research unless a separate licence is obtained.
func livenessModelBackend() string {
	backend := strings.ToLower(strings.TrimSpace(os.Getenv("FACESIGN_LIVENESS_BACKEND")))
	switch backend {
	case "mn3", "minifas":
		return backend
	default:
		return "silent"
	}
}

func livenessModelSpec() spec {
	switch livenessModelBackend() {
	case "mn3":
		return spec{Name: "anti-spoof-mn3.onnx", SHA256: "c4c99af04603b62d7e44f6f4daeb33e0daeccc696008c0b1d62f6f5cebbb3262"}
	case "minifas":
		return spec{Name: "minifasnet-v2se.onnx", SHA256: "af2381b88f38769222ed93379e12444e2a50814575de1c46170de570c55a42b6", URL: "https://github.com/facenox/face-antispoof-onnx/releases/download/v1.0.0/best_model.onnx"}
	default:
		return spec{Name: "MiniFASNetV2.onnx", SHA256: "b32929adc2d9c34b9486f8c4c7bc97c1b69bc0ea9befefc380e4faae4e463907", URL: "https://github.com/yakhyo/face-anti-spoofing/releases/download/weights/MiniFASNetV2.onnx"}
	}
}
func ResolveBy(assetsPath string) (Paths, error) {
	livenessSpec := livenessModelSpec()
	supportSpecs := []spec{livenessSpec}
	if livenessModelBackend() == "silent" {
		supportSpecs = append(supportSpecs, spec{Name: "MiniFASNetV1SE.onnx", SHA256: "ebab7f90c7833fbccd46d3a555410e78d969db5438e169b6524be444862b3676", URL: "https://github.com/yakhyo/face-anti-spoofing/releases/download/weights/MiniFASNetV1SE.onnx"})
	}
	supportSpecs = append(supportSpecs, spec{Name: "yolox_nano.onnx", SHA256: "c789161ed43c8269fcd4e67c67eeeb4e80c622da2eb296a20bc6007bd18a0b7d", URL: "https://github.com/Megvii-BaseDetection/YOLOX/releases/download/0.1.1rc0/yolox_nano.onnx"})
	support, err := resolve(assetsPath, candidateDirectories(assetsPath), defaultBaseURL, supportSpecs)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve liveness/person models: %w", err)
	}

	modelDirs := byModelDirectories(assetsPath)
	var modelDir string
	for _, dir := range modelDirs {
		if fileExists(filepath.Join(dir, bySCRFDFile)) && fileExists(filepath.Join(dir, byArcFaceFile)) {
			modelDir = dir
			break
		}
	}
	if modelDir == "" {
		return Paths{}, fmt.Errorf("By face models are missing; place the authorized %s and %s files in %s (or set FACESIGN_BY_MODEL_DIR)", bySCRFDFile, byArcFaceFile, filepath.Join(assetsPath, "models", "by"))
	}

	checksums, err := readByChecksums(filepath.Join(modelDir, "SHA256SUMS"))
	if err != nil {
		return Paths{}, err
	}
	for _, name := range []string{bySCRFDFile, byArcFaceFile} {
		expected := checksums[name]
		if expected == "" {
			return Paths{}, fmt.Errorf("SHA256SUMS must contain an entry for %s", name)
		}
		if !validModel(filepath.Join(modelDir, name), expected) {
			return Paths{}, fmt.Errorf("By model %s failed SHA-256 verification", name)
		}
	}
	paths := Paths{
		Detector: filepath.Join(modelDir, bySCRFDFile),
		Recognizer: filepath.Join(modelDir, byArcFaceFile),
		Liveness: filepath.Join(filepath.Dir(support.Liveness), livenessSpec.Name),
		Person: support.Person,
		Directory: modelDir,
		Backend: "by",
	}
	if name := optionalByModel(modelDir, byLandmark2DFile, checksums); name != "" {
		paths.Landmark2D = name
	}
	if name := optionalByModel(modelDir, byLandmark3DFile, checksums); name != "" {
		paths.Landmark3D = name
	}
	return paths, nil
}

func byModelDirectories(assetsPath string) []string {
	var dirs []string
	if custom := strings.TrimSpace(os.Getenv("FACESIGN_BY_MODEL_DIR")); custom != "" {
		dirs = append(dirs, custom)
	}
	dirs = append(dirs, filepath.Join(assetsPath, "models", "by"))
	if runtime.GOOS == "windows" {
		if programData := strings.TrimSpace(os.Getenv("ProgramData")); programData != "" {
			dirs = append(dirs, filepath.Join(programData, "FaceSign", "models", "by"))
		}
	}
	if cacheDir, err := os.UserCacheDir(); err == nil && cacheDir != "" {
		dirs = append(dirs, filepath.Join(cacheDir, "FaceSign", "models", "by"))
	}
	return uniqueCleanPaths(dirs)
}

func readByChecksums(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open By model SHA256SUMS %s: %w", path, err)
	}
	defer file.Close()
	out := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if filepath.Base(name) == name && len(fields[0]) == 64 {
			out[name] = strings.ToLower(fields[0])
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func optionalByModel(dir, name string, checksums map[string]string) string {
	path := filepath.Join(dir, name)
	if checksums[name] == "" || !fileExists(path) || !validModel(path, checksums[name]) {
		return ""
	}
	return path
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
