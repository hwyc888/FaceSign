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

	tlsIdentity, err := ensureTLSIdentity(cfg.TLSDir, cfg.TLSHosts)
	if err != nil {
		return fmt.Errorf("prepare HTTPS identity: %w", err)
	}
	logger.Info("FaceSign HTTPS identity ready", "root_ca", tlsIdentity.CACertPath, "tls_dir", cfg.TLSDir)

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

	appHandler := tlsBootstrapHandler(webServer.Handler(), tlsIdentity.CACertPath)
	httpHandler := http.Handler(appHandler)
	if cfg.HTTPRedirect {
		httpHandler = httpsRedirectHandler(appHandler, cfg.HTTPSListen)
	}
	httpServer := newHTTPServer(httpHandler)
	httpsServer := newHTTPServer(appHandler)

	httpsListener, err := net.Listen("tcp", cfg.HTTPSListen)
	if err != nil {
		return fmt.Errorf("listen for HTTPS on %s: %w", cfg.HTTPSListen, err)
	}
	defer httpsListener.Close()

	httpListener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen for HTTP on %s: %w; another FaceSign instance may still be running, so use scripts/install.ps1 as Administrator when upgrading", cfg.Listen, err)
	}
	defer httpListener.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 2)

	localURL := browserURL(cfg.Listen)
	if cfg.HTTPRedirect {
		localURL = secureBrowserURL(cfg.HTTPSListen)
	}
	writeStartupInfo(filepath.Dir(cfg.DataPath), httpListener.Addr().String(), httpsListener.Addr().String(), localURL, tlsIdentity.CACertPath)

	go func() {
		logger.Info("FaceSign HTTPS started", "version", version, "listen", httpsListener.Addr().String(), "url", secureBrowserURL(cfg.HTTPSListen), "database", cfg.DataPath, "device", "cpu")
		done <- httpsServer.ServeTLS(httpsListener, tlsIdentity.ServerCertPath, tlsIdentity.ServerKeyPath)
	}()
	go func() {
		logger.Info("FaceSign HTTP started", "listen", httpListener.Addr().String(), "redirect_to_https", cfg.HTTPRedirect)
		done <- httpServer.Serve(httpListener)
	}()

	if cfg.OpenBrowser {
		go func() {
			time.Sleep(350 * time.Millisecond)
			if err := openURL(localURL + "?v=" + version); err != nil {
				logger.Warn("could not open browser automatically", "url", localURL, "error", err)
			}
		}()
	}

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpsServer.Shutdown(shutdownCtx); err != nil {
			return err
		}
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	case err := <-done:
		_ = httpsServer.Close()
		_ = httpServer.Close()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func newHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
