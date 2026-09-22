package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hwyc888/FaceSign/internal/cameraio"
)

var version = "dev"

type config struct {
	ServerURL string `json:"server_url"`
	AgentID string `json:"agent_id"`
	AgentSecret string `json:"agent_secret"`
	FPS int `json:"fps"`
	ServerTLSInsecure bool `json:"server_tls_insecure"`
	Camera cameraio.Config `json:"camera"`
}

func main() {
	var configPath string
	var once, showVersion bool
	flag.StringVar(&configPath, "config", "camera-agent.json", "camera agent JSON config path")
	flag.BoolVar(&once, "once", false, "capture and upload one frame, then exit")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.Parse()
	if showVersion { fmt.Println(version); return }
	cfg, err := loadConfig(configPath)
	if err != nil { log.Fatal(err) }
	client := serverClient(cfg.ServerTLSInsecure)
	interval := time.Second / time.Duration(cfg.FPS)
	for {
		started := time.Now()
		err := captureAndUpload(context.Background(), client, cfg)
		if err != nil {
			log.Printf("camera upload failed: %v", err)
			if once { os.Exit(1) }
		} else if once {
			log.Printf("camera frame uploaded: agent=%s", cfg.AgentID)
			return
		}
		wait := interval - time.Since(started)
		if wait < 100*time.Millisecond { wait = 100 * time.Millisecond }
		time.Sleep(wait)
	}
}

func loadConfig(path string) (config, error) {
	if !filepath.IsAbs(path) {
		if exe, err := os.Executable(); err == nil {
			candidate := filepath.Join(filepath.Dir(exe), path)
			if _, err := os.Stat(candidate); err == nil { path = candidate }
		}
	}
	data, err := os.ReadFile(path)
	if err != nil { return config{}, fmt.Errorf("read config %s: %w", path, err) }
	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil { return config{}, fmt.Errorf("parse config: %w", err) }
	cfg.ServerURL = strings.TrimRight(strings.TrimSpace(cfg.ServerURL), "/")
	cfg.AgentID = strings.TrimSpace(cfg.AgentID)
	cfg.AgentSecret = strings.TrimSpace(cfg.AgentSecret)
	if cfg.FPS == 0 { cfg.FPS = 2 }
	if cfg.FPS < 1 || cfg.FPS > 10 { return config{}, errors.New("fps must be between 1 and 10") }
	u, err := url.Parse(cfg.ServerURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") { return config{}, errors.New("server_url must be a valid http/https URL") }
	if cfg.AgentID == "" || cfg.AgentSecret == "" { return config{}, errors.New("agent_id and agent_secret are required") }
	if cfg.Camera.Protocol == "" { cfg.Camera.Protocol = "http_snapshot" }
	if cfg.Camera.TimeoutMS == 0 { cfg.Camera.TimeoutMS = 3000 }
	return cfg, nil
}

func serverClient(insecure bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecure { transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} }
	return &http.Client{Transport: transport, Timeout: 15 * time.Second}
}

func captureAndUpload(ctx context.Context, client *http.Client, cfg config) error {
	frame, _, _, err := cameraio.FetchFrame(ctx, cfg.Camera)
	if err != nil { return fmt.Errorf("capture camera frame: %w", err) }
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.ServerURL+"/api/camera-agents/frame", bytes.NewReader(frame))
	if err != nil { return err }
	req.Header.Set("Content-Type", "image/jpeg")
	req.Header.Set("X-FaceSign-Agent-ID", cfg.AgentID)
	req.Header.Set("Authorization", "Bearer "+cfg.AgentSecret)
	req.Header.Set("User-Agent", "FaceSign-CameraAgent/"+version)
	response, err := client.Do(req)
	if err != nil { return fmt.Errorf("connect FaceSign server: %w", err) }
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("FaceSign server returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
