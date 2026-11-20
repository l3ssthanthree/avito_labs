package trip_test

import (
	"math"
	"testing"
	"time"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

func TestPositionValidation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		point domain.Coordinates
		at    time.Time
		valid bool
	}{
		{"zero coordinates", domain.Coordinates{}, now, true},
		{"boundaries", domain.Coordinates{Latitude: -90, Longitude: 180}, now, true},
		{"future boundary", domain.Coordinates{}, now.Add(time.Minute), true},
		{"old offline position", domain.Coordinates{}, now.Add(-24 * time.Hour), true},
		{"latitude", domain.Coordinates{Latitude: 90.01}, now, false},
		{"longitude", domain.Coordinates{Longitude: -180.01}, now, false},
		{"nan", domain.Coordinates{Latitude: math.NaN()}, now, false},
		{"infinity", domain.Coordinates{Longitude: math.Inf(1)}, now, false},
		{"oldest valid date", domain.Coordinates{}, time.Time{}, true},
		{"future", domain.Coordinates{}, now.Add(time.Minute + time.Nanosecond), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			input := domain.PositionInput{Coordinates: tc.point, RecordedAt: tc.at}
			if err := input.Validate(now); (err == nil) != tc.valid {
				t.Fatalf("valid=%v, got %v", tc.valid, err)
			}
		})
	}
}
