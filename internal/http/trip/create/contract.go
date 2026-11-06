//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

package create

import (
	"context"

	"github.com/google/uuid"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

type tripUsecase interface {
	Create(ctx context.Context, input domain.CreateInput, idempotencyKey *uuid.UUID) (domain.Trip, bool, error)
}
