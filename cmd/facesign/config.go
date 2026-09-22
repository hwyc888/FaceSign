package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

type config struct {
	Listen             string
	HTTPSListen        string
	TLSDir             string
	TLSHosts           string
	HTTPRedirect       bool
	DataPath           string
	AssetsPath         string
	MatchThreshold     float64
	DetectionThreshold float64
	OpenBrowser        bool
}

func loadConfig() (config, error) {
	exe, err := os.Executable()
	if err != nil {
		return config{}, err
	}
	base := filepath.Dir(exe)

	var cfg config
	flag.StringVar(&cfg.Listen, "listen", "0.0.0.0:8080", "HTTP listen address")
	flag.StringVar(&cfg.HTTPSListen, "https-listen", "0.0.0.0:8443", "HTTPS listen address")
	flag.StringVar(&cfg.TLSDir, "tls-dir", filepath.Join(base, "tls"), "directory containing the persistent FaceSign CA and server certificate")
	flag.StringVar(&cfg.TLSHosts, "tls-hosts", "", "additional comma-separated DNS names or IP addresses for the HTTPS certificate")
	flag.BoolVar(&cfg.HTTPRedirect, "http-redirect", false, "redirect HTTP requests to HTTPS except the root CA download")
	flag.StringVar(&cfg.DataPath, "data", filepath.Join(base, "data", "facesign.db"), "SQLite database path")
	flag.StringVar(&cfg.AssetsPath, "assets", base, "directory containing ONNX Runtime and models")
	flag.Float64Var(&cfg.MatchThreshold, "match-threshold", 0.68, "face match threshold from 0 to 1")
	flag.Float64Var(&cfg.DetectionThreshold, "detection-threshold", 0.80, "face detection threshold from 0 to 1")
	flag.BoolVar(&cfg.OpenBrowser, "open-browser", true, "open the local FaceSign page after startup")
	flag.Parse()

	if cfg.MatchThreshold <= 0 || cfg.MatchThreshold >= 1 {
		return config{}, fmt.Errorf("match-threshold must be between 0 and 1")
	}
	if cfg.DetectionThreshold <= 0 || cfg.DetectionThreshold >= 1 {
		return config{}, fmt.Errorf("detection-threshold must be between 0 and 1")
	}
	if cfg.Listen == "" {
		return config{}, fmt.Errorf("listen cannot be empty")
	}
	if cfg.HTTPSListen == "" {
		return config{}, fmt.Errorf("https-listen cannot be empty")
	}
	if cfg.TLSDir == "" {
		return config{}, fmt.Errorf("tls-dir cannot be empty")
	}
	return cfg, nil
}
