package trip_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

func TestTripFinish(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	finished := started.Add(time.Minute)
	for _, tc := range []struct {
		name       string
		status     trip.Status
		finishedAt *time.Time
		wantErr    error
		wantTime   time.Time
	}{
		{name: "active", status: trip.StatusActive, wantTime: finished},
		{name: "completed", status: trip.StatusCompleted, finishedAt: &started, wantErr: trip.ErrTripCompleted, wantTime: started},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			entity := trip.Restore(uuid.New(), trip.CreateInput{}, started, tc.status, tc.finishedAt)
			if err := entity.Finish(finished); !errors.Is(err, tc.wantErr) {
				t.Fatalf("Finish() = %v, want %v", err, tc.wantErr)
			}
			if at := entity.FinishedAt(); entity.Status() != trip.StatusCompleted || at == nil || !at.Equal(tc.wantTime) {
				t.Fatalf("status = %s, finished at = %v, want completed at %v", entity.Status(), at, tc.wantTime)
			}
		})
	}
}

func TestNewTripValidation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		change func(*trip.CreateInput)
		field  string
	}{
		{name: "zero price and coordinate boundaries"},
		{name: "missing user", change: func(in *trip.CreateInput) { in.UserID = uuid.Nil }, field: "user_id"},
		{name: "missing driver", change: func(in *trip.CreateInput) { in.DriverID = uuid.Nil }, field: "driver_id"},
		{name: "negative price", change: func(in *trip.CreateInput) { in.Price = -1 }, field: "price"},
		{name: "latitude out of bounds", change: func(in *trip.CreateInput) { in.StartPoint.Latitude = 91 }, field: "start_point.latitude"},
		{name: "longitude out of bounds", change: func(in *trip.CreateInput) { in.EndPoint.Longitude = -181 }, field: "end_point.longitude"},
		{name: "NaN latitude", change: func(in *trip.CreateInput) { in.EndPoint.Latitude = math.NaN() }, field: "end_point.latitude"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			input := trip.CreateInput{
				UserID: uuid.New(), DriverID: uuid.New(),
				StartPoint: trip.Coordinates{Latitude: -90, Longitude: 180},
				EndPoint:   trip.Coordinates{Latitude: 90, Longitude: -180},
			}
			if tc.change != nil {
				tc.change(&input)
			}
			entity, err := trip.New(uuid.New(), input, time.Now())
			if tc.field == "" {
				if err != nil || entity.Status() != trip.StatusActive || entity.FinishedAt() != nil {
					t.Fatalf("new active trip: %+v, error %v", entity, err)
				}
				return
			}
			var validation *trip.ValidationError
			if !errors.As(err, &validation) || validation.Field != tc.field || validation.Reason == "" {
				t.Fatalf("validation error = %v, want error for field %q", err, tc.field)
			}
		})
	}
}

func TestValidationErrorIncludesFieldAndReason(t *testing.T) {
	t.Parallel()
	err := &trip.ValidationError{Field: "start_point.latitude", Reason: "out of range"}
	if got := err.Error(); got != "start_point.latitude: out of range" {
		t.Fatalf("Error() = %q", got)
	}
}
