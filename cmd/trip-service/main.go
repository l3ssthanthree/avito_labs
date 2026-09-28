package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/l3ssthanthree/avito_labs/api"
	"github.com/l3ssthanthree/avito_labs/internal/config"
	"github.com/l3ssthanthree/avito_labs/internal/handler"
	"github.com/l3ssthanthree/avito_labs/internal/repository"
	"github.com/l3ssthanthree/avito_labs/internal/service"
	"github.com/l3ssthanthree/avito_labs/internal/txmanager"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load configuration", "error", err)
		os.Exit(1)
	}

	logLevel := new(slog.Level)
	if err := logLevel.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		slog.Error("parse log level", "value", cfg.LogLevel, "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})).With("service", "trip-service")
	slog.SetDefault(logger)

	databaseConfig, err := pgxpool.ParseConfig(cfg.Database.URL)
	if err != nil {
		logger.Error("parse database configuration", "error", err)
		os.Exit(1)
	}
	databaseConfig.MaxConns = cfg.Database.MaxConns
	databaseConfig.MinConns = cfg.Database.MinConns
	databaseConfig.MaxConnLifetime = cfg.Database.MaxConnLifetime

	connectCtx, cancelConnect := context.WithTimeout(context.Background(), cfg.Database.ConnectTimeout)
	database, err := pgxpool.NewWithConfig(connectCtx, databaseConfig)
	if err != nil {
		cancelConnect()
		logger.Error("create database pool", "error", err)
		os.Exit(1)
	}
	if err := database.Ping(connectCtx); err != nil {
		cancelConnect()
		database.Close()
		logger.Error("ping database", "error", err)
		os.Exit(1)
	}
	cancelConnect()
	defer database.Close()

	tripRepository := repository.NewTripRepository(database)
	transactionManager := txmanager.New(database)
	tripService := service.NewTripService(
		tripRepository, transactionManager,
		cfg.Database.QueryTimeout, cfg.IdempotencyKeyTTL,
	)
	httpHandler := handler.New(database, tripService, cfg.Database.QueryTimeout, logger)

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	api.HandlerWithOptions(httpHandler, api.ChiServerOptions{
		BaseRouter:       router,
		ErrorHandlerFunc: httpHandler.OpenAPIError,
	})

	server := &http.Server{
		Addr: cfg.HTTP.Addr, Handler: router,
		ReadTimeout: cfg.HTTP.ReadTimeout, ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout, IdleTimeout: cfg.HTTP.IdleTimeout,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("HTTP server started", "address", cfg.HTTP.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	select {
	case <-signalCtx.Done():
		logger.Info("shutdown signal received")
	case err := <-serverErrors:
		logger.Error("HTTP server failed", "error", err)
		return
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown HTTP server", "error", err)
		os.Exit(1)
	}
	logger.Info("HTTP server stopped")
}
