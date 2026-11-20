package container

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/l3ssthanthree/avito_labs/internal/config"
	"github.com/l3ssthanthree/avito_labs/internal/gateway/push"
	transport "github.com/l3ssthanthree/avito_labs/internal/http"
	"github.com/l3ssthanthree/avito_labs/internal/http/health"
	"github.com/l3ssthanthree/avito_labs/internal/http/middleware"
	positionlisthttp "github.com/l3ssthanthree/avito_labs/internal/http/position/list"
	positionsavehttp "github.com/l3ssthanthree/avito_labs/internal/http/position/save"
	"github.com/l3ssthanthree/avito_labs/internal/http/ready"
	"github.com/l3ssthanthree/avito_labs/internal/http/response"
	createhttp "github.com/l3ssthanthree/avito_labs/internal/http/trip/create"
	finishhttp "github.com/l3ssthanthree/avito_labs/internal/http/trip/finish"
	gethttp "github.com/l3ssthanthree/avito_labs/internal/http/trip/get"
	"github.com/l3ssthanthree/avito_labs/internal/infra/postgres"
	"github.com/l3ssthanthree/avito_labs/internal/infra/postgres/transaction"
	"github.com/l3ssthanthree/avito_labs/internal/infra/telemetry"
	repository "github.com/l3ssthanthree/avito_labs/internal/repository/trip"
	positionlist "github.com/l3ssthanthree/avito_labs/internal/usecase/position/list"
	positionsave "github.com/l3ssthanthree/avito_labs/internal/usecase/position/save"
	"github.com/l3ssthanthree/avito_labs/internal/usecase/trip/create"
	"github.com/l3ssthanthree/avito_labs/internal/usecase/trip/finish"
	"github.com/l3ssthanthree/avito_labs/internal/usecase/trip/get"
	"github.com/l3ssthanthree/avito_labs/internal/worker/positions"
)

type Container struct {
	Server        *http.Server
	Logger        *slog.Logger
	database      *pgxpool.Pool
	observability *telemetry.Telemetry
	positions     *positions.Pool
	push          *push.Gateway
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
	defer func() {
		if err != nil {
			database.Close()
		}
	}()
	pushGateway, err := push.New(cfg.Positions.PushURL, cfg.Positions.PushTimeout, observability.Traces)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			pushGateway.Close()
		}
	}()
	transactionManager := transaction.New(database)
	tripRepository := repository.New(transactionManager)
	worker, err := positions.New(tripRepository, pushGateway, cfg.Positions, cfg.Database.QueryTimeout,
		positions.RealClock{}, observability.Logger, observability.Traces, observability.Meters.Meter("tripgo/positions"))
	if err != nil {
		return nil, err
	}
	handler, err := newHTTPHandler(database, cfg, observability, transactionManager, tripRepository, worker)
	if err != nil {
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
		positions: worker, push: pushGateway,
	}, nil
}

func newHTTPHandler(database *pgxpool.Pool, cfg config.Config, observability *telemetry.Telemetry,
	transactionManager *transaction.Manager, tripRepository *repository.Repository, worker *positions.Pool,
) (http.Handler, error) {
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
	positionSaver := positionsave.New(tripRepository, transactionManager, worker, queryTimeout, time.Now)
	positionEvents, err := telemetry.NewPositionEvents(positionSaver, tripsMeter)
	if err != nil {
		return nil, err
	}
	positionReader := positionlist.New(tripRepository, queryTimeout)
	router, err := transport.NewRouter(transport.Endpoints{
		Save:      positionsavehttp.New(positionEvents, writer),
		List:      positionlisthttp.New(positionReader, writer),
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
	if err := c.positions.Stop(ctx); err != nil {
		return err
	}
	c.push.Close()
	err := c.observability.Close(ctx)
	c.database.Close()
	return err
}

func (c *Container) Start(ctx context.Context) { c.positions.Start(ctx) }
