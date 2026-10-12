package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type TripStatus string

const (
	TripStatusActive    TripStatus = "active"
	TripStatusCompleted TripStatus = "completed"
)

var (
	ErrInvalidRequest      = errors.New("invalid request")
	ErrTripNotFound        = errors.New("trip not found")
	ErrTripCompleted       = errors.New("trip already completed")
	ErrDriverBusy          = errors.New("driver already has an active trip")
	ErrIdempotencyConflict = errors.New("idempotency key was already used with a different request body")
)

type Coordinates struct {
	Latitude  float64
	Longitude float64
}

type CreateTripInput struct {
	UserID     uuid.UUID
	DriverID   uuid.UUID
	StartPoint Coordinates
	EndPoint   Coordinates
	Price      int64
}

type Trip struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	DriverID       uuid.UUID
	StartPoint     Coordinates
	EndPoint       Coordinates
	Price          int64
	Status         TripStatus
	StartedAt      time.Time
	FinishedAt     *time.Time
	LastPositionAt *time.Time
}

type IdempotencyRecord struct {
	RequestHash []byte
	TripID      uuid.UUID
}
