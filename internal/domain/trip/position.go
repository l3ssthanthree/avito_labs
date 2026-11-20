package trip

import (
	"time"

	"github.com/google/uuid"
)

type PositionInput struct {
	Coordinates Coordinates
	RecordedAt  time.Time
}

type Position struct {
	ID     int64
	TripID uuid.UUID
	PositionInput
}

type StaleTrip struct {
	ID       uuid.UUID
	DriverID uuid.UUID
	LastSeen time.Time
}

func (in PositionInput) Validate(now time.Time) error {
	if err := in.Coordinates.validate("coordinates"); err != nil {
		return err
	}
	if in.RecordedAt.After(now.Add(time.Minute)) {
		return &ValidationError{Field: "recorded_at", Reason: "must not be more than one minute in the future"}
	}
	return nil
}
