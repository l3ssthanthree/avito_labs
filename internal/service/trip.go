package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/l3ssthanthree/avito_labs/internal/model"
	"github.com/l3ssthanthree/avito_labs/internal/repository"
)

type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type TripService struct {
	repository        *repository.TripRepository
	txManager         TxManager
	queryTimeout      time.Duration
	idempotencyKeyTTL time.Duration
}

func NewTripService(
	repository *repository.TripRepository,
	txManager TxManager,
	queryTimeout time.Duration,
	idempotencyKeyTTL time.Duration,
) *TripService {
	return &TripService{
		repository:        repository,
		txManager:         txManager,
		queryTimeout:      queryTimeout,
		idempotencyKeyTTL: idempotencyKeyTTL,
	}
}

func (s *TripService) Create(ctx context.Context, input model.CreateTripInput, idempotencyKey *uuid.UUID) (model.Trip, bool, error) {
	if err := validateCreateInput(input); err != nil {
		return model.Trip{}, false, err
	}

	now := time.Now().UTC()
	trip := model.Trip{
		ID: uuid.New(), UserID: input.UserID, DriverID: input.DriverID,
		StartPoint: input.StartPoint, EndPoint: input.EndPoint,
		Price: input.Price, Status: model.TripStatusActive, StartedAt: now,
	}

	queryCtx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	var replayed bool
	err := s.txManager.Do(queryCtx, func(txCtx context.Context) error {
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
					return model.ErrIdempotencyConflict
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
		return s.repository.AddStatusHistory(txCtx, trip.ID, nil, model.TripStatusActive, nil)
	})
	if err != nil {
		return model.Trip{}, false, fmt.Errorf("create trip: %w", err)
	}
	return trip, replayed, nil
}

func hashCreateInput(input model.CreateTripInput) ([]byte, error) {
	canonicalBody, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal create trip input for idempotency: %w", err)
	}
	hash := sha256.Sum256(canonicalBody)
	return hash[:], nil
}

func (s *TripService) Get(ctx context.Context, tripID uuid.UUID) (model.Trip, error) {
	queryCtx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	trip, err := s.repository.Get(queryCtx, tripID)
	if err != nil {
		return model.Trip{}, fmt.Errorf("get trip: %w", err)
	}
	return trip, nil
}

func (s *TripService) Finish(ctx context.Context, tripID uuid.UUID) (model.Trip, error) {
	queryCtx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	var trip model.Trip
	err := s.txManager.Do(queryCtx, func(txCtx context.Context) error {
		updatedTrip, err := s.repository.Finish(txCtx, tripID, time.Now().UTC())
		if err != nil {
			return err
		}

		fromStatus := model.TripStatusActive
		if err := s.repository.AddStatusHistory(txCtx, tripID, &fromStatus, model.TripStatusCompleted, nil); err != nil {
			return err
		}
		trip = updatedTrip
		return nil
	})
	if err != nil {
		return model.Trip{}, fmt.Errorf("finish trip: %w", err)
	}
	return trip, nil
}

func validateCreateInput(input model.CreateTripInput) error {
	if input.UserID == uuid.Nil {
		return fmt.Errorf("%w: user_id must not be empty", model.ErrInvalidRequest)
	}
	if input.DriverID == uuid.Nil {
		return fmt.Errorf("%w: driver_id must not be empty", model.ErrInvalidRequest)
	}
	if err := validateCoordinates("start_point", input.StartPoint); err != nil {
		return err
	}
	if err := validateCoordinates("end_point", input.EndPoint); err != nil {
		return err
	}
	if input.Price < 0 {
		return fmt.Errorf("%w: price must not be negative", model.ErrInvalidRequest)
	}
	return nil
}

func validateCoordinates(name string, coordinates model.Coordinates) error {
	if coordinates.Latitude < -90 || coordinates.Latitude > 90 {
		return fmt.Errorf("%w: %s latitude is out of range", model.ErrInvalidRequest, name)
	}
	if coordinates.Longitude < -180 || coordinates.Longitude > 180 {
		return fmt.Errorf("%w: %s longitude is out of range", model.ErrInvalidRequest, name)
	}
	return nil
}
