package main

import (
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"runtime"
)

const ortVersion = "1.26.0"

func browserURL(listen string) string {
	return browserURLForScheme(listen, "http")
}

func secureBrowserURL(listen string) string {
	return browserURLForScheme(listen, "https")
}

func browserURLForScheme(listen, scheme string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		if scheme == "https" {
			return "https://127.0.0.1:8443/"
		}
		return "http://127.0.0.1:8080/"
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return scheme + "://" + net.JoinHostPort(host, port) + "/"
}

func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("open browser is not supported on %s", runtime.GOOS)
	}
	return cmd.Start()
}

func runtimeLibraryPath(assets string) (string, error) {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(assets, "onnxruntime.dll"), nil
	case "linux":
		return filepath.Join(assets, "libonnxruntime.so."+ortVersion), nil
	default:
		return "", fmt.Errorf("unsupported operating system %s", runtime.GOOS)
	}
}
