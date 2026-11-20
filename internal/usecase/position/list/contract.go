//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

package list

import (
	"context"

	"github.com/google/uuid"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

type positionRepository interface {
	Get(ctx context.Context, id uuid.UUID) (domain.Trip, error)
	ListPositions(ctx context.Context, id uuid.UUID) ([]domain.Position, error)
}
