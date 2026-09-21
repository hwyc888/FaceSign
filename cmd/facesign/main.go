package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/hwyc888/FaceSign/internal/face"
	"github.com/hwyc888/FaceSign/internal/store"
	webapp "github.com/hwyc888/FaceSign/internal/web"
)

const ortVersion = "1.26.0"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("FaceSign stopped", "error", err)
		writeStartupError(err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	base := filepath.Dir(exe)
	listen := flag.String("listen", "0.0.0.0:8080", "HTTP listen address")
	dataPath := flag.String("data", filepath.Join(base, "data", "facesign.db"), "SQLite database path")
	assetsPath := flag.String("assets", base, "directory containing ONNX Runtime and models")
	matchThreshold := flag.Float64("match-threshold", 0.68, "face match threshold from 0 to 1")
	detectionThreshold := flag.Float64("detection-threshold", 0.80, "face detection threshold from 0 to 1")
	openBrowser := flag.Bool("open-browser", true, "open the local FaceSign page after startup")
	flag.Parse()

	if *matchThreshold <= 0 || *matchThreshold >= 1 {
		return fmt.Errorf("match-threshold must be between 0 and 1")
	}
	if *detectionThreshold <= 0 || *detectionThreshold >= 1 {
		return fmt.Errorf("detection-threshold must be between 0 and 1")
	}

	runtimePath, err := runtimeLibraryPath(*assetsPath)
	if err != nil {
		return err
	}
	detectorModel := filepath.Join(*assetsPath, "models", "face_detection_yunet_2023mar.onnx")
	recognizerModel := filepath.Join(*assetsPath, "models", "face_recognition_sface_2021dec.onnx")

	st, err := store.Open(*dataPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer st.Close()

	engine, err := face.New(runtimePath, detectorModel, recognizerModel)
	if err != nil {
		return fmt.Errorf("start face engine: %w", err)
	}
	defer engine.Close()

	webServer, err := webapp.New(logger, st, engine, *matchThreshold, *detectionThreshold)
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler:           webServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", *listen, err)
	}
	defer listener.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	localURL := browserURL(*listen)
	go func() {
		logger.Info("FaceSign started", "listen", listener.Addr().String(), "browser", localURL, "database", *dataPath, "device", "cpu")
		done <- server.Serve(listener)
	}()

	if *openBrowser {
		go func() {
			time.Sleep(250 * time.Millisecond)
			if err := openURL(localURL); err != nil {
				logger.Warn("could not open browser automatically", "url", localURL, "error", err)
			}
		}()
	}

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		err := <-done
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func browserURL(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "http://127.0.0.1:8080/"
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/"
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
