package list

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

type Usecase struct {
	repository   positionRepository
	queryTimeout time.Duration
}

func New(repository positionRepository, timeout time.Duration) *Usecase {
	return &Usecase{repository: repository, queryTimeout: timeout}
}

func (u *Usecase) List(ctx context.Context, id uuid.UUID) ([]domain.Position, error) {
	queryCtx, cancel := context.WithTimeout(ctx, u.queryTimeout)
	defer cancel()
	if _, err := u.repository.Get(queryCtx, id); err != nil {
		return nil, fmt.Errorf("get trip for route: %w", err)
	}
	positions, err := u.repository.ListPositions(queryCtx, id)
	if err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}
	return positions, nil
}
