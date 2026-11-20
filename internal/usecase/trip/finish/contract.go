//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

package finish

import (
	"context"

	"github.com/google/uuid"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

type tripRepository interface {
	Get(ctx context.Context, tripID uuid.UUID) (domain.Trip, error)
	SaveFinished(ctx context.Context, trip domain.Trip) (domain.Trip, error)
	AddStatusHistory(ctx context.Context, tripID uuid.UUID, fromStatus *domain.Status, toStatus domain.Status, reason *string) error
}

type txManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}
