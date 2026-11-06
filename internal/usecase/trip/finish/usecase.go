package finish

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

type Usecase struct {
	repository   tripRepository
	txManager    txManager
	queryTimeout time.Duration
}

func New(repository tripRepository, txManager txManager, queryTimeout time.Duration) *Usecase {
	return &Usecase{repository: repository, txManager: txManager, queryTimeout: queryTimeout}
}

func (u *Usecase) Finish(ctx context.Context, tripID uuid.UUID) (domain.Trip, error) {
	queryCtx, cancel := context.WithTimeout(ctx, u.queryTimeout)
	defer cancel()

	var trip domain.Trip
	err := u.txManager.Do(queryCtx, func(txCtx context.Context) error {
		currentTrip, err := u.repository.Get(txCtx, tripID)
		if err != nil {
			return err
		}

		fromStatus := currentTrip.Status()
		if err := currentTrip.Finish(time.Now().UTC()); err != nil {
			return err
		}
		updatedTrip, err := u.repository.SaveFinished(txCtx, currentTrip)
		if err != nil {
			return err
		}
		if err := u.repository.AddStatusHistory(txCtx, tripID, &fromStatus, updatedTrip.Status(), nil); err != nil {
			return err
		}
		trip = updatedTrip
		return nil
	})
	if err != nil {
		return domain.Trip{}, fmt.Errorf("finish trip: %w", err)
	}
	return trip, nil
}
