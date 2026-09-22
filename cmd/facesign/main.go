package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hwyc888/FaceSign/internal/face"
	"github.com/hwyc888/FaceSign/internal/liveness"
	"github.com/hwyc888/FaceSign/internal/models"
	"github.com/hwyc888/FaceSign/internal/store"
	webapp "github.com/hwyc888/FaceSign/internal/web"
)

var version = "dev"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("FaceSign stopped", "error", err)
		writeStartupError(err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	runtimePath, err := runtimeLibraryPath(cfg.AssetsPath)
	if err != nil {
		return err
	}
	modelPaths, err := models.Resolve(cfg.AssetsPath)
	if err != nil {
		return fmt.Errorf("prepare face models: %w", err)
	}
	logger.Info("FaceSign models ready", "directory", modelPaths.Directory)

	st, err := store.Open(cfg.DataPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer st.Close()

	engine, err := face.New(runtimePath, modelPaths.Detector, modelPaths.Recognizer)
	if err != nil {
		return fmt.Errorf("start face engine: %w", err)
	}
	defer engine.Close()

	livenessEngine, err := liveness.New(modelPaths.Liveness)
	if err != nil {
		return fmt.Errorf("start passive liveness engine: %w", err)
	}
	defer livenessEngine.Close()

	webServer, err := webapp.New(logger, st, engine, livenessEngine, cfg.MatchThreshold, cfg.DetectionThreshold, version)
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

	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w; another FaceSign instance may still be running, so use scripts/install.ps1 as Administrator when upgrading", cfg.Listen, err)
	}
	defer listener.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	localURL := browserURL(cfg.Listen)
	writeStartupInfo(filepath.Dir(cfg.DataPath), listener.Addr().String(), localURL)
	go func() {
		logger.Info("FaceSign started", "version", version, "listen", listener.Addr().String(), "browser", localURL, "database", cfg.DataPath, "device", "cpu")
		done <- server.Serve(listener)
	}()

	if cfg.OpenBrowser {
		go func() {
			time.Sleep(250 * time.Millisecond)
			if err := openURL(localURL + "?v=" + version); err != nil {
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
