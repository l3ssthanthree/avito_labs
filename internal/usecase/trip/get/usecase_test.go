package get_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
	"github.com/l3ssthanthree/avito_labs/internal/usecase/trip/get"
)

func TestGet(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	for _, tc := range []struct {
		name string
		trip domain.Trip
		err  error
	}{
		{name: "found", trip: domain.Restore(id, domain.CreateInput{}, time.Now(), domain.StatusActive, nil)},
		{name: "not found", err: domain.ErrTripNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := NewMocktripRepository(gomock.NewController(t))
			repo.EXPECT().Get(gomock.Any(), id).Return(tc.trip, tc.err)
			got, err := get.New(repo, time.Second).Get(t.Context(), id)
			if !errors.Is(err, tc.err) || got.ID != tc.trip.ID {
				t.Fatalf("Get() = (%v, %v), want (%v, %v)", got.ID, err, tc.trip.ID, tc.err)
			}
		})
	}
}
