package create

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

type Usecase struct {
	repository        tripRepository
	txManager         txManager
	queryTimeout      time.Duration
	idempotencyKeyTTL time.Duration
}

func New(
	repository tripRepository,
	txManager txManager,
	queryTimeout time.Duration,
	idempotencyKeyTTL time.Duration,
) *Usecase {
	return &Usecase{
		repository:        repository,
		txManager:         txManager,
		queryTimeout:      queryTimeout,
		idempotencyKeyTTL: idempotencyKeyTTL,
	}
}

func (s *Usecase) Create(ctx context.Context, input domain.CreateInput, idempotencyKey *uuid.UUID) (domain.Trip, bool, error) {
	now := time.Now().UTC()
	trip, err := domain.New(uuid.New(), input, now)
	if err != nil {
		return domain.Trip{}, false, err
	}

	queryCtx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	var replayed bool
	err = s.txManager.Do(queryCtx, func(txCtx context.Context) error {
		if idempotencyKey != nil {
			requestHash, err := hashCreateInput(input)
			if err != nil {
				return err
			}
			if err := s.repository.DeleteExpiredIdempotencyKey(txCtx, *idempotencyKey, now); err != nil {
				return err
			}

			reserved, err := s.repository.ReserveIdempotencyKey(
				txCtx, *idempotencyKey, requestHash, trip.ID, now.Add(s.idempotencyKeyTTL),
			)
			if err != nil {
				return err
			}
			if !reserved {
				record, err := s.repository.GetIdempotencyRecord(txCtx, *idempotencyKey)
				if err != nil {
					return err
				}
				if !bytes.Equal(record.RequestHash, requestHash) {
					return domain.ErrIdempotencyConflict
				}
				trip, err = s.repository.Get(txCtx, record.TripID)
				if err != nil {
					return err
				}
				replayed = true
				return nil
			}
		}

		if err := s.repository.Create(txCtx, trip); err != nil {
			return err
		}
		return s.repository.AddStatusHistory(txCtx, trip.ID, nil, domain.StatusActive, nil)
	})
	if err != nil {
		return domain.Trip{}, false, fmt.Errorf("create trip: %w", err)
	}
	return trip, replayed, nil
}

func hashCreateInput(input domain.CreateInput) ([]byte, error) {
	canonicalBody, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal create trip input for idempotency: %w", err)
	}
	hash := sha256.Sum256(canonicalBody)
	return hash[:], nil
}
