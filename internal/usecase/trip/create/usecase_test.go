package create_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
	"github.com/l3ssthanthree/avito_labs/internal/model"
	"github.com/l3ssthanthree/avito_labs/internal/usecase/trip/create"
)

type txKey struct{}

func TestCreate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "created"},
		{name: "busy driver", err: domain.ErrDriverBusy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			input := domain.CreateInput{UserID: uuid.New(), DriverID: uuid.New()}
			repo, tx, txCtx := mockTransaction(t)

			var savedID uuid.UUID
			writeTrip := repo.EXPECT().Create(txCtx, gomock.Any()).DoAndReturn(func(_ context.Context, value domain.Trip) error {
				if value.ID == uuid.Nil || value.CreateInput != input || value.Status() != domain.StatusActive {
					t.Fatalf("unexpected trip: %+v", value)
				}
				savedID = value.ID
				return tc.err
			})
			if tc.err == nil {
				sameID := gomock.Cond(func(id uuid.UUID) bool { return id == savedID })
				repo.EXPECT().AddStatusHistory(txCtx, sameID, nil, domain.StatusActive, nil).After(writeTrip)
			}

			entity, replayed, err := create.New(repo, tx, time.Second, time.Hour).Create(t.Context(), input, nil)
			if !errors.Is(err, tc.err) || replayed {
				t.Fatalf("Create() replayed = %v, error = %v, want false, %v", replayed, err, tc.err)
			}
			if err == nil && entity.ID != savedID {
				t.Fatalf("created ID = %v, want %v", entity.ID, savedID)
			}
		})
	}
}

func TestCreateReusedKey(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		wantErr error
	}{
		{name: "replayed"},
		{name: "different body", wantErr: domain.ErrIdempotencyConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			input := domain.CreateInput{UserID: uuid.New(), DriverID: uuid.New(), Price: 300}
			stored := domain.Restore(uuid.New(), input, time.Now(), domain.StatusActive, nil)
			key := uuid.New()
			body, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(body)
			record := model.IdempotencyRecord{TripID: stored.ID, RequestHash: hash[:]}
			if tc.wantErr != nil {
				record.RequestHash = []byte("hash of a different body")
			}

			repo, tx, txCtx := mockTransaction(t)
			gomock.InOrder(
				repo.EXPECT().DeleteExpiredIdempotencyKey(txCtx, key, gomock.Any()),
				repo.EXPECT().ReserveIdempotencyKey(txCtx, key, hash[:], gomock.Any(), gomock.Any()).Return(false, nil),
				repo.EXPECT().GetIdempotencyRecord(txCtx, key).Return(record, nil),
			)
			if tc.wantErr == nil {
				repo.EXPECT().Get(txCtx, stored.ID).Return(stored, nil)
			}

			entity, replayed, err := create.New(repo, tx, time.Second, time.Hour).Create(t.Context(), input, &key)
			if !errors.Is(err, tc.wantErr) || replayed != (tc.wantErr == nil) {
				t.Fatalf("Create() replayed = %v, error = %v, want error %v", replayed, err, tc.wantErr)
			}
			if replayed && entity.ID != stored.ID {
				t.Fatalf("replayed ID = %v, want %v", entity.ID, stored.ID)
			}
		})
	}
}

func mockTransaction(t *testing.T) (*MocktripRepository, *MocktxManager, context.Context) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	txCtx := context.WithValue(t.Context(), txKey{}, true)
	tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, run func(context.Context) error) error { return run(txCtx) },
	)
	return repo, tx, txCtx
}
