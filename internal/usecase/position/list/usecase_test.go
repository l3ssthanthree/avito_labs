package list_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
	"github.com/l3ssthanthree/avito_labs/internal/usecase/position/list"
)

func TestListPositions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name            string
		getErr, listErr error
	}{
		{"empty route", nil, nil}, {"missing", domain.ErrTripNotFound, nil}, {"query failed", nil, errors.New("db failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := NewMockpositionRepository(gomock.NewController(t))
			id := uuid.New()
			repo.EXPECT().Get(gomock.Any(), id).Return(domain.Trip{}, tc.getErr)
			if tc.getErr == nil {
				repo.EXPECT().ListPositions(gomock.Any(), id).Return([]domain.Position{}, tc.listErr)
			}
			_, err := list.New(repo, time.Second).List(t.Context(), id)
			want := tc.getErr
			if want == nil {
				want = tc.listErr
			}
			if !errors.Is(err, want) {
				t.Fatal(err)
			}
		})
	}
}
