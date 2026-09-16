package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr                   string
	DatabasePath           string
	Timezone               string
	SessionTTL             time.Duration
	KioskAccessKey         string
	FaceProvider           string
	CompreFaceURL          string
	CompreFaceAPIKey       string
	FaceSimilarity         float64
	FaceDetectionThreshold float64
	TLSCertFile            string
	TLSKeyFile             string
	CookieSecure           bool
}

func Load() (Config, error) {
	if err := loadEnvironmentFile(); err != nil {
		return Config{}, err
	}
	cfg := Config{
		Addr:             env("FACESIGN_ADDR", ":8080"),
		DatabasePath:     env("FACESIGN_DB", "./data/facesign.db"),
		Timezone:         env("FACESIGN_TIMEZONE", "Asia/Shanghai"),
		KioskAccessKey:   os.Getenv("FACESIGN_KIOSK_ACCESS_KEY"),
		FaceProvider:     strings.ToLower(env("FACESIGN_FACE_PROVIDER", "disabled")),
		CompreFaceURL:    strings.TrimRight(env("FACESIGN_COMPREFACE_URL", "http://127.0.0.1:8000"), "/"),
		CompreFaceAPIKey: os.Getenv("FACESIGN_COMPREFACE_API_KEY"),
		TLSCertFile:      os.Getenv("FACESIGN_TLS_CERT"),
		TLSKeyFile:       os.Getenv("FACESIGN_TLS_KEY"),
	}

	hours, err := strconv.Atoi(env("FACESIGN_SESSION_HOURS", "12"))
	if err != nil || hours < 1 || hours > 168 {
		return Config{}, fmt.Errorf("FACESIGN_SESSION_HOURS must be between 1 and 168")
	}
	cfg.SessionTTL = time.Duration(hours) * time.Hour

	cfg.FaceSimilarity, err = parseFloat("FACESIGN_FACE_SIMILARITY", 0.78, 0, 1)
	if err != nil {
		return Config{}, err
	}
	cfg.FaceDetectionThreshold, err = parseFloat("FACESIGN_FACE_DETECTION_THRESHOLD", 0.80, 0, 1)
	if err != nil {
		return Config{}, err
	}

	cfg.CookieSecure, err = strconv.ParseBool(env("FACESIGN_COOKIE_SECURE", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("FACESIGN_COOKIE_SECURE must be true or false")
	}
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return Config{}, fmt.Errorf("FACESIGN_TLS_CERT and FACESIGN_TLS_KEY must be configured together")
	}
	if cfg.TLSCertFile != "" {
		cfg.CookieSecure = true
	}
	if _, err := time.LoadLocation(cfg.Timezone); err != nil {
		return Config{}, fmt.Errorf("invalid FACESIGN_TIMEZONE: %w", err)
	}
	if cfg.FaceProvider == "compreface" && cfg.CompreFaceAPIKey == "" {
		return Config{}, fmt.Errorf("FACESIGN_COMPREFACE_API_KEY is required when FACESIGN_FACE_PROVIDER=compreface")
	}
	if cfg.FaceProvider != "disabled" && cfg.FaceProvider != "compreface" {
		return Config{}, fmt.Errorf("unsupported FACESIGN_FACE_PROVIDER %q", cfg.FaceProvider)
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func parseFloat(key string, fallback, min, max float64) (float64, error) {
	value := env(key, strconv.FormatFloat(fallback, 'f', -1, 64))
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < min || parsed > max {
		return 0, fmt.Errorf("%s must be between %.2f and %.2f", key, min, max)
	}
	return parsed, nil
}

func loadEnvironmentFile() error {
	if explicit := strings.TrimSpace(os.Getenv("FACESIGN_ENV_FILE")); explicit != "" {
		return readEnvironmentFile(explicit, true)
	}
	candidates := make([]string, 0, 2)
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "facesign.env"))
	}
	candidates = append(candidates, "facesign.env")
	seen := map[string]bool{}
	for _, candidate := range candidates {
		abs, _ := filepath.Abs(candidate)
		if seen[abs] {
			continue
		}
		seen[abs] = true
		if err := readEnvironmentFile(candidate, false); err != nil {
			return err
		}
	}
	return nil
}

func readEnvironmentFile(path string, required bool) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) && !required {
			return nil
		}
		return fmt.Errorf("open environment file %s: %w", path, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		key, value, ok := strings.Cut(text, "=")
		if !ok {
			return fmt.Errorf("invalid environment file %s line %d", path, line)
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), "\"'")
		if key == "" {
			return fmt.Errorf("invalid environment file %s line %d", path, line)
		}
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("set %s from environment file: %w", key, err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read environment file %s: %w", path, err)
	}
	return nil
}
