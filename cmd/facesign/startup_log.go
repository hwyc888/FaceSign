package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func writeStartupInfo(dir, httpListen, httpsListen, browserURL, caPath string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	message := fmt.Sprintf(
		"started=%s\r\nversion=%s\r\npid=%d\r\nhttp_listen=%s\r\nhttps_listen=%s\r\nurl=%s\r\nroot_ca=%s\r\n",
		time.Now().Format(time.RFC3339), version, os.Getpid(), httpListen, httpsListen, browserURL, caPath,
	)
	_ = os.WriteFile(filepath.Join(dir, "facesign-startup.log"), []byte(message), 0o644)
}

func writeStartupError(startupErr error) {
	message := time.Now().Format(time.RFC3339) + " " + startupErr.Error() + "\r\n"
	exe, err := os.Executable()
	if err == nil {
		path := filepath.Join(filepath.Dir(exe), "facesign-error.log")
		if err := os.WriteFile(path, []byte(message), 0o644); err == nil {
			return
		}
	}
	_ = os.WriteFile(filepath.Join(os.TempDir(), "facesign-error.log"), []byte(message), 0o644)
}
