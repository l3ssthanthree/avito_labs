package container

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/l3ssthanthree/avito_labs/internal/config"
	transport "github.com/l3ssthanthree/avito_labs/internal/http"
	"github.com/l3ssthanthree/avito_labs/internal/http/health"
	"github.com/l3ssthanthree/avito_labs/internal/http/middleware"
	"github.com/l3ssthanthree/avito_labs/internal/http/ready"
	"github.com/l3ssthanthree/avito_labs/internal/http/response"
	createhttp "github.com/l3ssthanthree/avito_labs/internal/http/trip/create"
	finishhttp "github.com/l3ssthanthree/avito_labs/internal/http/trip/finish"
	gethttp "github.com/l3ssthanthree/avito_labs/internal/http/trip/get"
	"github.com/l3ssthanthree/avito_labs/internal/infra/postgres"
	"github.com/l3ssthanthree/avito_labs/internal/infra/postgres/transaction"
	"github.com/l3ssthanthree/avito_labs/internal/infra/telemetry"
	repository "github.com/l3ssthanthree/avito_labs/internal/repository/trip"
	"github.com/l3ssthanthree/avito_labs/internal/usecase/trip/create"
	"github.com/l3ssthanthree/avito_labs/internal/usecase/trip/finish"
	"github.com/l3ssthanthree/avito_labs/internal/usecase/trip/get"
)

type Container struct {
	Server        *http.Server
	Logger        *slog.Logger
	database      *pgxpool.Pool
	observability *telemetry.Telemetry
}

func New(ctx context.Context, cfg config.Config) (_ *Container, err error) {
	observability, err := telemetry.New(ctx, cfg.Telemetry, cfg.LogLevel)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
			defer cancel()
			err = errors.Join(err, observability.Close(closeCtx))
		}
	}()

	tracer, err := telemetry.NewPostgresTracer(observability.Traces, observability.Meters.Meter("tripgo/postgres"))
	if err != nil {
		return nil, err
	}
	database, err := postgres.Open(ctx, cfg.Database, tracer)
	if err != nil {
		return nil, err
	}
	handler, err := newHTTPHandler(database, cfg, observability)
	if err != nil {
		database.Close()
		return nil, err
	}
	return &Container{
		Server: &http.Server{
			Addr:              cfg.HTTP.Addr,
			Handler:           handler,
			ReadTimeout:       cfg.HTTP.ReadTimeout,
			ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
			WriteTimeout:      cfg.HTTP.WriteTimeout,
			IdleTimeout:       cfg.HTTP.IdleTimeout,
		},
		Logger: observability.Logger, database: database, observability: observability,
	}, nil
}

func newHTTPHandler(database *pgxpool.Pool, cfg config.Config, observability *telemetry.Telemetry) (http.Handler, error) {
	transactionManager := transaction.New(database)
	tripRepository := repository.New(transactionManager)
	queryTimeout := cfg.Database.QueryTimeout
	createTrip := create.New(tripRepository, transactionManager, queryTimeout, cfg.IdempotencyKeyTTL)
	getTrip := get.New(tripRepository, queryTimeout)
	finishTrip := finish.New(tripRepository, transactionManager, queryTimeout)

	tripsMeter := observability.Meters.Meter("tripgo/trips")
	events, err := telemetry.NewTripEvents(createTrip, finishTrip, observability.Logger, tripsMeter)
	if err != nil {
		return nil, err
	}
	writer := response.New(observability.Logger)
	router, err := transport.NewRouter(transport.Endpoints{
		Create:    createhttp.New(events, writer),
		Get:       gethttp.New(getTrip, writer),
		Finish:    finishhttp.New(events, writer),
		Liveness:  health.New(writer),
		Readiness: ready.New(database, queryTimeout, writer),
	}, writer)
	if err != nil {
		return nil, err
	}

	handler, err := middleware.New(router, observability.Logger, observability.Traces, observability.Meters.Meter("tripgo/http"), writer.Panic)
	if err != nil {
		return nil, err
	}
	if err := telemetry.ObserveActiveTrips(tripsMeter, func(ctx context.Context) (int64, error) {
		countCtx, cancel := context.WithTimeout(ctx, queryTimeout)
		defer cancel()
		return tripRepository.CountActive(countCtx)
	}); err != nil {
		return nil, err
	}
	return handler, nil
}

func (c *Container) Close(ctx context.Context) error {
	err := c.observability.Close(ctx)
	c.database.Close()
	return err
}
