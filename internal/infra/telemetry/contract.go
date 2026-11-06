package telemetry

import (
	"context"

	"github.com/google/uuid"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

type tripCreator interface {
	Create(ctx context.Context, input domain.CreateInput, key *uuid.UUID) (domain.Trip, bool, error)
}

type tripFinisher interface {
	Finish(ctx context.Context, id uuid.UUID) (domain.Trip, error)
}
