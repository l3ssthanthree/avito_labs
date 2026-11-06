package get

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

type Usecase struct {
	repository   tripRepository
	queryTimeout time.Duration
}

func New(repository tripRepository, queryTimeout time.Duration) *Usecase {
	return &Usecase{repository: repository, queryTimeout: queryTimeout}
}

func (u *Usecase) Get(ctx context.Context, tripID uuid.UUID) (domain.Trip, error) {
	queryCtx, cancel := context.WithTimeout(ctx, u.queryTimeout)
	defer cancel()

	trip, err := u.repository.Get(queryCtx, tripID)
	if err != nil {
		return domain.Trip{}, fmt.Errorf("get trip: %w", err)
	}
	return trip, nil
}
