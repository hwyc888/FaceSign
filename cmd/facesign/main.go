package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/hwyc888/FaceSign/internal/app"
	"github.com/hwyc888/FaceSign/internal/config"
	"github.com/hwyc888/FaceSign/internal/face"
	"github.com/hwyc888/FaceSign/internal/servicehost"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if len(os.Args) > 1 && os.Args[1] == "provision-compreface" {
		if err := runCompreFaceProvision(os.Args[2:]); err != nil {
			logger.Error("CompreFace provisioning failed", "error", err)
			os.Exit(1)
		}
		return
	}
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

func runCompreFaceProvision(args []string) error {
	flags := flag.NewFlagSet("provision-compreface", flag.ContinueOnError)
	serviceURL := flags.String("url", "http://127.0.0.1:8000", "CompreFace base URL")
	email := flags.String("email", strings.TrimSpace(os.Getenv("COMPREFACE_ADMIN_EMAIL")), "CompreFace administrator email")
	if err := flags.Parse(args); err != nil {
		return err
	}
	password := os.Getenv("COMPREFACE_ADMIN_PASSWORD")
	if strings.TrimSpace(*email) == "" || password == "" {
		return fmt.Errorf("请通过 --email/COMPREFACE_ADMIN_EMAIL 和 COMPREFACE_ADMIN_PASSWORD 提供 CompreFace 管理员凭据")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	result, err := face.ProvisionCompreFace(ctx, face.CompreFaceProvisionOptions{
		BaseURL:  *serviceURL,
		Email:    *email,
		Password: password,
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, result.APIKey)
	return nil
}
