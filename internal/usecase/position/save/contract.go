//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

package save

import (
	"context"

	"github.com/google/uuid"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

type positionRepository interface {
	SavePosition(ctx context.Context, id uuid.UUID, input domain.PositionInput) (domain.Position, error)
}

type txManager interface {
	Do(ctx context.Context, fn func(context.Context) error) error
}

type pendingChecks interface {
	Cancel(ctx context.Context, id uuid.UUID)
}
