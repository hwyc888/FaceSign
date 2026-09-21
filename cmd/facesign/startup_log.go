package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func writeStartupInfo(dir, listen, localURL string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	message := fmt.Sprintf(
		"started=%s\r\nversion=%s\r\npid=%d\r\nlisten=%s\r\nurl=%s\r\n",
		time.Now().Format(time.RFC3339), version, os.Getpid(), listen, localURL,
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
