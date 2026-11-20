package save

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

type Usecase struct {
	repository   positionRepository
	txManager    txManager
	pending      pendingChecks
	queryTimeout time.Duration
	now          func() time.Time
}

func New(repository positionRepository, manager txManager, pending pendingChecks, timeout time.Duration, now func() time.Time) *Usecase {
	return &Usecase{repository: repository, txManager: manager, pending: pending, queryTimeout: timeout, now: now}
}

func (u *Usecase) Save(ctx context.Context, id uuid.UUID, input domain.PositionInput) (domain.Position, error) {
	if err := input.Validate(u.now()); err != nil {
		return domain.Position{}, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, u.queryTimeout)
	defer cancel()
	var position domain.Position
	err := u.txManager.Do(queryCtx, func(txCtx context.Context) error {
		var err error
		position, err = u.repository.SavePosition(txCtx, id, input)
		return err
	})
	if err != nil {
		return domain.Position{}, fmt.Errorf("save position: %w", err)
	}
	u.pending.Cancel(ctx, id)
	return position, nil
}
