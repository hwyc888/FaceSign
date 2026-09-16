package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/hwyc888/FaceSign/internal/app"
	"github.com/hwyc888/FaceSign/internal/config"
	"github.com/hwyc888/FaceSign/internal/servicehost"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration error", "error", err)
		os.Exit(1)
	}
	if err := servicehost.Run(logger, func(ctx context.Context) error { return app.Run(ctx, cfg, logger) }); err != nil {
		logger.Error("FaceSign stopped with error", "error", err)
		os.Exit(1)
	}
}
