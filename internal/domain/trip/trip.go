package trip

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
)

type Trip struct {
	ID uuid.UUID
	CreateInput
	StartedAt      time.Time
	LastPositionAt *time.Time
	status         Status
	finishedAt     *time.Time
}

func New(id uuid.UUID, input CreateInput, startedAt time.Time) (Trip, error) {
	if err := input.Validate(); err != nil {
		return Trip{}, err
	}
	return Restore(id, input, startedAt, StatusActive, nil), nil
}

func Restore(id uuid.UUID, input CreateInput, startedAt time.Time, status Status, finishedAt *time.Time) Trip {
	if finishedAt != nil {
		at := *finishedAt
		finishedAt = &at
	}
	return Trip{ID: id, CreateInput: input, StartedAt: startedAt, status: status, finishedAt: finishedAt}
}

func (t Trip) Status() Status {
	return t.status
}

func (t Trip) FinishedAt() *time.Time {
	if t.finishedAt == nil {
		return nil
	}
	at := *t.finishedAt
	return &at
}

func (t *Trip) Finish(at time.Time) error {
	if t.status != StatusActive {
		return ErrTripCompleted
	}
	t.status = StatusCompleted
	t.finishedAt = &at
	return nil
}
