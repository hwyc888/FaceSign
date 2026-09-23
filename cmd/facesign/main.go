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
	"github.com/hwyc888/FaceSign/internal/person"
	"github.com/hwyc888/FaceSign/internal/store"
	webapp "github.com/hwyc888/FaceSign/internal/web"
)

var version = "dev"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("FaceSign stopped", "error", err)
		writeStartupError(err)
		showStartupFailure(err)
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
		return fmt.Errorf("准备 HTTPS 证书失败: %w", err)
	}
	logger.Info("FaceSign HTTPS identity ready", "root_ca", tlsIdentity.CACertPath, "tls_dir", cfg.TLSDir)

	runtimePath, err := runtimeLibraryPath(cfg.AssetsPath)
	if err != nil {
		return err
	}
	modelPaths, err := models.Resolve(cfg.AssetsPath)
	if err != nil {
		return fmt.Errorf("准备人脸模型失败: %w", err)
	}
	logger.Info("FaceSign models ready", "directory", modelPaths.Directory)

	st, err := store.Open(cfg.DataPath)
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}
	defer st.Close()

	engine, err := face.New(runtimePath, modelPaths.Detector, modelPaths.Recognizer)
	if err != nil {
		return fmt.Errorf("启动人脸识别引擎失败: %w", err)
	}
	defer engine.Close()

	livenessEngine, err := liveness.New(modelPaths.Liveness)
	if err != nil {
		return fmt.Errorf("启动活体检测引擎失败: %w", err)
	}
	defer livenessEngine.Close()

	personEngine, err := person.New(modelPaths.Person)
	if err != nil {
		return fmt.Errorf("启动人体检测引擎失败: %w", err)
	}
	defer personEngine.Close()

	webServer, err := webapp.New(logger, st, engine, livenessEngine, personEngine, cfg.MatchThreshold, cfg.DetectionThreshold, version)
	if err != nil {
		return err
	}
	defer webServer.Close()

	appHandler := tlsBootstrapHandler(webServer.Handler(), tlsIdentity.CACertPath)
	httpHandler := http.Handler(appHandler)
	if cfg.HTTPRedirect {
		httpHandler = httpsRedirectHandler(appHandler, cfg.HTTPSListen)
	}
	httpServer := newHTTPServer(httpHandler)
	httpsServer := newHTTPServer(appHandler)

	httpListener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("HTTP 端口 %s 无法监听: %w；很可能已有 FaceSign 或其他程序占用了 8080。升级已安装版本请以管理员身份运行 scripts/install.ps1，不要直接双击新的 EXE", cfg.Listen, err)
	}
	defer httpListener.Close()

	var httpsListener net.Listener
	httpsListener, err = net.Listen("tcp", cfg.HTTPSListen)
	if err != nil {
		if cfg.HTTPRedirect {
			return fmt.Errorf("HTTPS 端口 %s 无法监听: %w；服务器模式必须启用 HTTPS，请检查 8443 是否被旧 FaceSign、IIS 或其他程序占用", cfg.HTTPSListen, err)
		}
		logger.Warn("HTTPS port unavailable; local portable mode will continue on HTTP", "listen", cfg.HTTPSListen, "error", err)
		httpsListener = nil
	} else {
		defer httpsListener.Close()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 2)

	localURL := browserURL(cfg.Listen)
	if cfg.HTTPRedirect && httpsListener != nil {
		localURL = secureBrowserURL(cfg.HTTPSListen)
	}
	httpsListenLog := "disabled"
	if httpsListener != nil {
		httpsListenLog = httpsListener.Addr().String()
	}
	writeStartupInfo(filepath.Dir(cfg.DataPath), httpListener.Addr().String(), httpsListenLog, localURL, tlsIdentity.CACertPath)

	if httpsListener != nil {
		go func() {
			logger.Info("FaceSign HTTPS started", "version", version, "listen", httpsListener.Addr().String(), "url", secureBrowserURL(cfg.HTTPSListen), "database", cfg.DataPath, "device", "cpu")
			done <- httpsServer.ServeTLS(httpsListener, tlsIdentity.ServerCertPath, tlsIdentity.ServerKeyPath)
		}()
	}
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
		if httpsListener != nil {
			if err := httpsServer.Shutdown(shutdownCtx); err != nil {
				return err
			}
		}
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	case err := <-done:
		if httpsListener != nil {
			_ = httpsServer.Close()
		}
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
