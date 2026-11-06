package telemetry

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

type TripEvents struct {
	creator   tripCreator
	finisher  tripFinisher
	logger    *slog.Logger
	created   metric.Int64Counter
	completed metric.Int64Counter
}

func NewTripEvents(creator tripCreator, finisher tripFinisher, logger *slog.Logger, meter metric.Meter) (*TripEvents, error) {
	created, err := meter.Int64Counter("tripgo.trips.created", metric.WithUnit("{trip}"))
	if err != nil {
		return nil, fmt.Errorf("create trips created counter: %w", err)
	}
	completed, err := meter.Int64Counter("tripgo.trips.completed", metric.WithUnit("{trip}"))
	if err != nil {
		return nil, fmt.Errorf("create trips completed counter: %w", err)
	}
	return &TripEvents{creator: creator, finisher: finisher, logger: logger, created: created, completed: completed}, nil
}

func (e *TripEvents) Create(ctx context.Context, input domain.CreateInput, key *uuid.UUID) (domain.Trip, bool, error) {
	entity, replayed, err := e.creator.Create(ctx, input, key)
	if err == nil && !replayed {
		e.created.Add(ctx, 1)
		e.logger.InfoContext(ctx, "trip created", "trip_id", entity.ID)
	}
	return entity, replayed, err
}

func (e *TripEvents) Finish(ctx context.Context, id uuid.UUID) (domain.Trip, error) {
	entity, err := e.finisher.Finish(ctx, id)
	if err == nil {
		e.completed.Add(ctx, 1)
		e.logger.InfoContext(ctx, "trip completed", "trip_id", entity.ID)
	}
	return entity, err
}

func ObserveActiveTrips(meter metric.Meter, count func(context.Context) (int64, error)) error {
	_, err := meter.Int64ObservableGauge("tripgo.trips.active",
		metric.WithUnit("{trip}"),
		metric.WithInt64Callback(func(ctx context.Context, observer metric.Int64Observer) error {
			active, err := count(ctx)
			if err != nil {
				return err
			}
			observer.Observe(active)
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("create active trips gauge: %w", err)
	}
	return nil
}
