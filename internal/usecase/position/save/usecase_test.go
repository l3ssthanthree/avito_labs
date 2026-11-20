package save_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
	"github.com/l3ssthanthree/avito_labs/internal/usecase/position/save"
)

func TestSavePosition(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		failure error
	}{
		{"saved", nil},
		{"missing trip", domain.ErrTripNotFound},
		{"completed trip", domain.ErrTripCompleted},
		{"repository failed", errors.New("db failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			repo, tx, pending := NewMockpositionRepository(ctrl), NewMocktxManager(ctrl), NewMockpendingChecks(ctrl)
			now, id := time.Now(), uuid.New()
			input := domain.PositionInput{RecordedAt: now}
			expected := domain.Position{ID: 1, TripID: id, PositionInput: input}
			tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) })
			store := repo.EXPECT().SavePosition(gomock.Any(), id, input).Return(expected, tc.failure)
			if tc.failure == nil {
				pending.EXPECT().Cancel(gomock.Any(), id).After(store)
			}
			uc := save.New(repo, tx, pending, time.Second, func() time.Time { return now })
			got, err := uc.Save(t.Context(), id, input)
			if !errors.Is(err, tc.failure) {
				t.Fatalf("got %v, want %v", err, tc.failure)
			}
			if err == nil && got != expected {
				t.Fatalf("position %+v, want %+v", got, expected)
			}
		})
	}
}

func TestSaveCommitFailureAndValidation(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo, tx, pending := NewMockpositionRepository(ctrl), NewMocktxManager(ctrl), NewMockpendingChecks(ctrl)
	now := time.Now()
	uc := save.New(repo, tx, pending, time.Second, func() time.Time { return now })
	if _, err := uc.Save(t.Context(), uuid.New(), domain.PositionInput{RecordedAt: now.Add(time.Minute + time.Second)}); err == nil {
		t.Fatal("invalid input accepted")
	}
	failure := errors.New("commit failed")
	tx.EXPECT().Do(gomock.Any(), gomock.Any()).Return(failure)
	if _, err := uc.Save(t.Context(), uuid.New(), domain.PositionInput{RecordedAt: now}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
}
