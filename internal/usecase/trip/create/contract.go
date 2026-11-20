//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

package create

import (
	"context"
	"time"

	"github.com/google/uuid"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
	"github.com/l3ssthanthree/avito_labs/internal/model"
)

type tripRepository interface {
	Create(ctx context.Context, trip domain.Trip) error
	AddStatusHistory(ctx context.Context, tripID uuid.UUID, fromStatus *domain.Status, toStatus domain.Status, reason *string) error
	Get(ctx context.Context, tripID uuid.UUID) (domain.Trip, error)
	DeleteExpiredIdempotencyKey(ctx context.Context, key uuid.UUID, now time.Time) error
	ReserveIdempotencyKey(ctx context.Context, key uuid.UUID, requestHash []byte, tripID uuid.UUID, expiresAt time.Time) (bool, error)
	GetIdempotencyRecord(ctx context.Context, key uuid.UUID) (model.IdempotencyRecord, error)
}

type txManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}
