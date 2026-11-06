package finish_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
	"github.com/l3ssthanthree/avito_labs/internal/usecase/trip/finish"
)

type txKey struct{}

func TestFinish(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		saveErr    error
		historyErr error
		completed  bool
		wantErr    error
	}{
		{name: "active trip"},
		{name: "already completed", completed: true, wantErr: domain.ErrTripCompleted},
		{name: "concurrent winner", saveErr: domain.ErrTripCompleted, wantErr: domain.ErrTripCompleted},
		{name: "history failure", historyErr: domain.ErrTripNotFound, wantErr: domain.ErrTripNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			tx := NewMocktxManager(ctrl)
			id := uuid.New()
			started := time.Now()
			entity := domain.Restore(id, domain.CreateInput{}, started, domain.StatusActive, nil)
			txCtx := context.WithValue(t.Context(), txKey{}, true)
			tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, run func(context.Context) error) error { return run(txCtx) },
			)

			if tc.completed {
				if err := entity.Finish(started.Add(time.Minute)); err != nil {
					t.Fatal(err)
				}
				repo.EXPECT().Get(txCtx, id).Return(entity, nil)
			} else {
				expectFinishWrites(t, repo, txCtx, entity, tc.saveErr, tc.historyErr)
			}

			got, err := finish.New(repo, tx, time.Second).Finish(t.Context(), id)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Finish() error = %v, want %v", err, tc.wantErr)
			}
			if err == nil && (got.ID != id || got.Status() != domain.StatusCompleted || got.FinishedAt() == nil) {
				t.Fatalf("unexpected finished trip: %+v", got)
			}
		})
	}
}

func expectFinishWrites(t *testing.T, repo *MocktripRepository, ctx context.Context, entity domain.Trip, saveErr, historyErr error) {
	t.Helper()
	getCall := repo.EXPECT().Get(ctx, entity.ID).Return(entity, nil)
	completed := gomock.Cond(func(value domain.Trip) bool {
		return value.ID == entity.ID && value.Status() == domain.StatusCompleted && value.FinishedAt() != nil
	})
	saveCall := repo.EXPECT().SaveFinished(ctx, completed).After(getCall).DoAndReturn(
		func(_ context.Context, value domain.Trip) (domain.Trip, error) {
			return value, saveErr
		},
	)
	if saveErr == nil {
		active := domain.StatusActive
		repo.EXPECT().AddStatusHistory(ctx, entity.ID, &active, domain.StatusCompleted, nil).After(saveCall).Return(historyErr)
	}
}
