package telemetry

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

type PositionEvents struct {
	saver positionSaver
	saved metric.Int64Counter
}

func NewPositionEvents(saver positionSaver, meter metric.Meter) (*PositionEvents, error) {
	saved, err := meter.Int64Counter("tripgo.positions.saved")
	if err != nil {
		return nil, fmt.Errorf("create positions saved counter: %w", err)
	}
	return &PositionEvents{saver: saver, saved: saved}, nil
}

func (e *PositionEvents) Save(ctx context.Context, id uuid.UUID, input domain.PositionInput) (domain.Position, error) {
	position, err := e.saver.Save(ctx, id, input)
	if err == nil {
		e.saved.Add(ctx, 1)
	}
	return position, err
}
