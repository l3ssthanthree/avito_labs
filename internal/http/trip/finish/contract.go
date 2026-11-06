//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

package finish

import (
	"context"

	"github.com/google/uuid"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

type tripUsecase interface {
	Finish(ctx context.Context, tripID uuid.UUID) (domain.Trip, error)
}
