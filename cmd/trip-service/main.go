package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/l3ssthanthree/avito_labs/internal/config"
	"github.com/l3ssthanthree/avito_labs/internal/container"
)

func main() {
	os.Exit(run(context.Background()))
}

func run(ctx context.Context) int {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "trip-service")
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		return 1
	}

	signalCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	app, err := container.New(signalCtx, cfg)
	if err != nil {
		logger.Error("initialize service", "error", err)
		return 1
	}

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(signalCtx, "tcp", app.Server.Addr)
	if err != nil {
		app.Logger.Error("listen for HTTP requests", "error", err)
		closeApp(ctx, app, cfg.ShutdownTimeout)
		return 1
	}

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- app.Server.Serve(listener)
	}()
	app.Logger.Info("HTTP server started", "address", listener.Addr().String())

	select {
	case <-signalCtx.Done():
		app.Logger.Info("shutdown signal received")
	case err := <-serverErrors:
		closeApp(ctx, app, cfg.ShutdownTimeout)
		if !errors.Is(err, http.ErrServerClosed) {
			app.Logger.Error("HTTP server failed", "error", err)
			return 1
		}
		return 0
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(ctx, cfg.ShutdownTimeout)
	defer cancelShutdown()
	if err := app.Server.Shutdown(shutdownCtx); err != nil {
		app.Logger.Error("shutdown HTTP server", "error", err)
		return 1
	}
	if err := app.Close(shutdownCtx); err != nil {
		app.Logger.Warn("flush telemetry", "error", err)
	}
	app.Logger.Info("HTTP server stopped")
	return 0
}

func closeApp(ctx context.Context, app *container.Container, timeout time.Duration) {
	closeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := app.Close(closeCtx); err != nil {
		app.Logger.Warn("flush telemetry", "error", err)
	}
}
