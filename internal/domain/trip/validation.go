package trip

import (
	"math"

	"github.com/google/uuid"
)

type Coordinates struct {
	Latitude  float64
	Longitude float64
}

type CreateInput struct {
	UserID     uuid.UUID
	DriverID   uuid.UUID
	StartPoint Coordinates
	EndPoint   Coordinates
	Price      int64
}

func (in CreateInput) Validate() error {
	if in.UserID == uuid.Nil {
		return &ValidationError{Field: "user_id", Reason: "must not be empty"}
	}
	if in.DriverID == uuid.Nil {
		return &ValidationError{Field: "driver_id", Reason: "must not be empty"}
	}
	if err := in.StartPoint.validate("start_point"); err != nil {
		return err
	}
	if err := in.EndPoint.validate("end_point"); err != nil {
		return err
	}
	if in.Price < 0 {
		return &ValidationError{Field: "price", Reason: "must not be negative"}
	}
	return nil
}

func (p Coordinates) validate(field string) error {
	const outOfRange = "out of range"
	if math.IsNaN(p.Latitude) || p.Latitude < -90 || p.Latitude > 90 {
		return &ValidationError{Field: field + ".latitude", Reason: outOfRange}
	}
	if math.IsNaN(p.Longitude) || p.Longitude < -180 || p.Longitude > 180 {
		return &ValidationError{Field: field + ".longitude", Reason: outOfRange}
	}
	return nil
}
