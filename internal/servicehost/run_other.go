//go:build !windows

package servicehost

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func Run(logger *slog.Logger, runner func(context.Context) error) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runner(ctx)
}
