//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

package save

import (
	"context"

	"github.com/google/uuid"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

type positionUsecase interface {
	Save(ctx context.Context, id uuid.UUID, input domain.PositionInput) (domain.Position, error)
}
