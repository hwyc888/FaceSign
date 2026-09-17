package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/hwyc888/FaceSign/internal/faceengine"
	"github.com/hwyc888/FaceSign/internal/servicehost"
)

const (
	serviceName = "FaceSignFaceEngine"
	ortVersion  = "1.26.0"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("face engine stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	base := filepath.Dir(exe)

	listen := flag.String("listen", "127.0.0.1:18081", "HTTP listen address")
	assets := flag.String("assets", base, "directory containing ONNX Runtime and models")
	data := flag.String("data", filepath.Join(base, "data", "faces.db"), "persistent face embedding SQLite database")
	flag.Parse()

	return servicehost.RunNamed(logger, serviceName, func(ctx context.Context) error {
		return serve(ctx, logger, *listen, *assets, *data)
	})
}

func serve(ctx context.Context, logger *slog.Logger, listen, assets, dataPath string) error {
	runtimePath, err := runtimeLibraryPath(assets)
	if err != nil {
		return err
	}
	detectorModel := filepath.Join(assets, "models", "face_detection_yunet_2023mar.onnx")
	recognizerModel := filepath.Join(assets, "models", "face_recognition_sface_2021dec.onnx")

	engine, err := faceengine.New(runtimePath, detectorModel, recognizerModel, dataPath)
	if err != nil {
		return err
	}
	defer engine.Close()

	server := &http.Server{
		Addr:              listen,
		Handler:           faceengine.NewHTTPHandler(engine),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	done := make(chan error, 1)
	go func() {
		logger.Info("native CPU face engine started", "listen", listen, "runtime", runtimePath, "device", "cpu")
		done <- server.ListenAndServe()
	}()

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

func runtimeLibraryPath(assets string) (string, error) {
	var name string
	switch runtime.GOOS {
	case "windows":
		name = "onnxruntime.dll"
	case "linux":
		name = "libonnxruntime.so." + ortVersion
	default:
		return "", fmt.Errorf("unsupported operating system %s", runtime.GOOS)
	}
	return filepath.Join(assets, name), nil
}
