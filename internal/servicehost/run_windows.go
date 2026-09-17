//go:build windows

package servicehost

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/windows/svc"
)

const serviceName = "FaceSign"

type windowsHandler struct {
	logger *slog.Logger
	runner func(context.Context) error
}

func Run(logger *slog.Logger, runner func(context.Context) error) error {
	return RunNamed(logger, serviceName, runner)
}

func RunNamed(logger *slog.Logger, name string, runner func(context.Context) error) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if !isService {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return runner(ctx)
	}
	return svc.Run(name, &windowsHandler{logger: logger, runner: runner})
}

func (h *windowsHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const accepts = svc.AcceptStop | svc.AcceptShutdown
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- h.runner(ctx) }()
	status <- svc.Status{State: svc.Running, Accepts: accepts}

	for {
		select {
		case err := <-result:
			status <- svc.Status{State: svc.StopPending}
			if err != nil {
				h.logger.Error("service stopped with error", "error", err)
				return false, 1
			}
			return false, 0
		case change := <-requests:
			switch change.Cmd {
			case svc.Interrogate:
				status <- svc.Status{State: svc.Running, Accepts: accepts}
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				err := <-result
				if err != nil {
					h.logger.Error("service shutdown failed", "error", err)
					return false, 1
				}
				return false, 0
			}
		}
	}
}
