package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/zanel1u/cloud-cli-proxy/internal/controlplane/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := app.Config{
		Addr:           envOrDefault("CONTROL_PLANE_ADDR", ":8080"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		MigrationDir:   "internal/store/migrations",
		AdminUsername:   envOrDefault("ADMIN_USERNAME", "admin"),
		AdminPassword:  os.Getenv("ADMIN_PASSWORD"),
		AdminJWTSecret: os.Getenv("ADMIN_JWT_SECRET"),

		SSHProxyAddr:              envOrDefault("SSH_PROXY_ADDR", ":2222"),
		SSHProxyContainerUser:     envOrDefault("SSH_PROXY_CONTAINER_USER", "workspace"),
		SSHProxyContainerPassword: envOrDefault("SSH_PROXY_CONTAINER_PASSWORD", "workspace"),
	}

	if cfg.DatabaseURL == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}

	if cfg.AdminJWTSecret == "" {
		slog.Warn("ADMIN_JWT_SECRET not set, admin API disabled")
	}

	controlPlane, err := app.New(ctx, cfg)
	if err != nil {
		slog.Error("failed to initialize control-plane", "error", err)
		os.Exit(1)
	}

	if err := controlPlane.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("control-plane stopped with error", "error", err)
		os.Exit(1)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
