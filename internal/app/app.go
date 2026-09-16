package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/hwyc888/FaceSign/internal/attendance"
	"github.com/hwyc888/FaceSign/internal/config"
	"github.com/hwyc888/FaceSign/internal/database"
	"github.com/hwyc888/FaceSign/internal/face"
	"github.com/hwyc888/FaceSign/internal/httpapi"
	"github.com/hwyc888/FaceSign/internal/realtime"
	"github.com/hwyc888/FaceSign/internal/repository"
)

func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	db, err := database.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return fmt.Errorf("load timezone: %w", err)
	}

	repo := repository.New(db)
	hub := realtime.NewHub()
	var faceProvider face.Provider = face.Disabled{}
	if cfg.FaceProvider == "compreface" {
		faceProvider = face.NewCompreFace(cfg.CompreFaceURL, cfg.CompreFaceAPIKey, cfg.FaceSimilarity, cfg.FaceDetectionThreshold)
	}
	attendanceService := attendance.New(repo, faceProvider, hub, location, logger)
	api := httpapi.New(cfg, repo, attendanceService, hub, logger, location)

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}

	schedulerCtx, cancelScheduler := context.WithCancel(ctx)
	defer cancelScheduler()
	go attendanceService.StartScheduler(schedulerCtx)

	errCh := make(chan error, 1)
	go func() {
		logger.Info("FaceSign server starting", "address", cfg.Addr, "timezone", cfg.Timezone, "face_provider", faceProvider.Name(), "tls", cfg.TLSCertFile != "")
		if cfg.TLSCertFile != "" {
			errCh <- server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
			return
		}
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
