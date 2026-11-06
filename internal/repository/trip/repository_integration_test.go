//go:build integration

package trip_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
	"github.com/l3ssthanthree/avito_labs/internal/infra/postgres/transaction"
	repository "github.com/l3ssthanthree/avito_labs/internal/repository/trip"
	"github.com/l3ssthanthree/avito_labs/internal/usecase/trip/create"
	"github.com/l3ssthanthree/avito_labs/internal/usecase/trip/finish"
)

const databaseTimeout = 10 * time.Second

func TestRollbackTripAndHistory(t *testing.T) {
	t.Parallel()
	pool := isolatedPool(t)
	manager := transaction.New(pool)
	repo := repository.New(manager)
	entity := newTrip(t)

	ctx, cancel := context.WithTimeout(t.Context(), databaseTimeout)
	defer cancel()
	err := manager.Do(ctx, func(txCtx context.Context) error {
		if err := repo.Create(txCtx, entity); err != nil {
			return err
		}
		if err := repo.AddStatusHistory(txCtx, entity.ID, nil, domain.StatusActive, nil); err != nil {
			return err
		}
		return repo.AddStatusHistory(txCtx, entity.ID, nil, "invalid-status", nil)
	})
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != "23514" {
		t.Fatalf("want PostgreSQL check constraint failure, got %v", err)
	}
	if _, err := repo.Get(ctx, entity.ID); !errors.Is(err, domain.ErrTripNotFound) {
		t.Fatalf("trip survived rollback or lookup failed: %v", err)
	}
	assertHistoryCount(t, ctx, pool, entity.ID, 0)
}

func TestConcurrentFinish(t *testing.T) {
	t.Parallel()
	pool := isolatedPool(t)
	manager := transaction.New(pool)
	repo := repository.New(manager)
	entity := newTrip(t)
	ctx, cancel := context.WithTimeout(t.Context(), databaseTimeout)
	defer cancel()
	if err := repo.Create(ctx, entity); err != nil {
		t.Fatal(err)
	}
	conflict := domain.Restore(uuid.New(), entity.CreateInput, entity.StartedAt, domain.StatusActive, nil)
	if err := repo.Create(ctx, conflict); !errors.Is(err, domain.ErrDriverBusy) {
		t.Fatalf("second active trip for the same driver: %v", err)
	}

	release := make(chan struct{})
	reads := make(chan struct{}, 2)
	results := make(chan error, 2)
	usecase := finish.New(&synchronizedRepo{Repository: repo, reads: reads, release: release}, manager, databaseTimeout)
	for range 2 {
		go func() {
			_, err := usecase.Finish(ctx, entity.ID)
			results <- err
		}()
	}
	for range 2 {
		select {
		case <-reads:
		case <-ctx.Done():
			t.Fatal("both transactions must read the active trip before updating")
		}
	}
	close(release)

	var succeeded, completed int
	for range 2 {
		select {
		case err := <-results:
			switch {
			case err == nil:
				succeeded++
			case errors.Is(err, domain.ErrTripCompleted):
				completed++
			default:
				t.Fatalf("unexpected finish error: %v", err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if succeeded != 1 || completed != 1 {
		t.Fatalf("got %d successful and %d conflicted finishes, want one each", succeeded, completed)
	}
	got, err := repo.Get(ctx, entity.ID)
	if err != nil || got.Status() != domain.StatusCompleted || got.FinishedAt() == nil {
		t.Fatalf("unexpected persisted trip: %+v, error %v", got, err)
	}
	assertHistoryCount(t, ctx, pool, entity.ID, 1)
}

type synchronizedRepo struct {
	*repository.Repository
	reads   chan<- struct{}
	release <-chan struct{}
}

func (r *synchronizedRepo) Get(ctx context.Context, id uuid.UUID) (domain.Trip, error) {
	entity, err := r.Repository.Get(ctx, id)
	if err != nil {
		return domain.Trip{}, err
	}
	r.reads <- struct{}{}
	select {
	case <-r.release:
		return entity, nil
	case <-ctx.Done():
		return domain.Trip{}, ctx.Err()
	}
}

func assertHistoryCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, want int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM trip_status_history WHERE trip_id = $1", id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("history rows = %d, want %d", count, want)
	}
}

func TestConcurrentCreateWithSameKey(t *testing.T) {
	t.Parallel()
	pool := isolatedPool(t)
	manager := transaction.New(pool)
	repo := repository.New(manager)
	ctx, cancel := context.WithTimeout(t.Context(), databaseTimeout)
	defer cancel()

	input := domain.CreateInput{UserID: uuid.New(), DriverID: uuid.New()}
	key := uuid.New()
	release := make(chan struct{})
	arrived := make(chan struct{}, 2)
	uc := create.New(&idempotencyBarrier{Repository: repo, arrived: arrived, release: release}, manager, databaseTimeout, time.Hour)
	type result struct {
		trip     domain.Trip
		replayed bool
		err      error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			entity, replayed, err := uc.Create(ctx, input, &key)
			results <- result{entity, replayed, err}
		}()
	}
	for range 2 {
		select {
		case <-arrived:
		case <-ctx.Done():
			t.Fatal("both transactions must reach idempotency processing")
		}
	}
	close(release)

	var got [2]result
	for i := range got {
		select {
		case got[i] = <-results:
			if got[i].err != nil {
				t.Fatal(got[i].err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	id := got[0].trip.ID
	if id == uuid.Nil || id != got[1].trip.ID || got[0].replayed == got[1].replayed {
		t.Fatalf("expected one creation and one replay of the same trip, got %+v", got)
	}
	assertHistoryCount(t, ctx, pool, id, 1)
	var trips int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM trips").Scan(&trips); err != nil || trips != 1 {
		t.Fatalf("got %d persisted trips, error %v", trips, err)
	}
}

type idempotencyBarrier struct {
	*repository.Repository
	arrived chan<- struct{}
	release <-chan struct{}
}

func (r *idempotencyBarrier) DeleteExpiredIdempotencyKey(ctx context.Context, key uuid.UUID, now time.Time) error {
	r.arrived <- struct{}{}
	select {
	case <-r.release:
		return r.Repository.DeleteExpiredIdempotencyKey(ctx, key, now)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestExpiredIdempotencyKeyCanBeReused(t *testing.T) {
	t.Parallel()
	pool := isolatedPool(t)
	manager := transaction.New(pool)
	repo := repository.New(manager)
	uc := create.New(repo, manager, databaseTimeout, time.Hour)
	ctx, cancel := context.WithTimeout(t.Context(), databaseTimeout)
	defer cancel()

	input := domain.CreateInput{UserID: uuid.New(), DriverID: uuid.New()}
	key := uuid.New()
	first, replayed, err := uc.Create(ctx, input, &key)
	if err != nil || replayed {
		t.Fatalf("first create: replayed %v, error %v", replayed, err)
	}
	input.DriverID = uuid.New()
	if _, _, err := uc.Create(ctx, input, &key); !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("different input with a live key: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE trip_create_idempotency
		SET created_at = now() - interval '2 hours', expires_at = now() - interval '1 hour'
		WHERE idempotency_key = $1`, key); err != nil {
		t.Fatal(err)
	}
	second, replayed, err := uc.Create(ctx, input, &key)
	if err != nil || replayed || second.ID == first.ID {
		t.Fatalf("reuse of expired key: trip %v, replayed %v, error %v", second.ID, replayed, err)
	}
	record, err := repo.GetIdempotencyRecord(ctx, key)
	if err != nil || record.TripID != second.ID {
		t.Fatalf("key points to %v, want %v, error %v", record.TripID, second.ID, err)
	}
	if _, err := repo.Get(ctx, first.ID); err != nil {
		t.Fatalf("original trip must remain available: %v", err)
	}
	assertHistoryCount(t, ctx, pool, first.ID, 1)
	assertHistoryCount(t, ctx, pool, second.ID, 1)
}

func newTrip(t *testing.T) domain.Trip {
	t.Helper()
	entity, err := domain.New(uuid.New(), domain.CreateInput{UserID: uuid.New(), DriverID: uuid.New()}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return entity
}

func isolatedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(t.Context(), databaseTimeout)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)

	schema := "tripgo_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), databaseTimeout)
		defer cancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 4
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	migrations, err := goose.NewProvider(goose.DialectPostgres, stdlib.OpenDBFromPool(pool), os.DirFS("../../../migrations"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := migrations.Close(); err != nil {
			t.Errorf("close migration provider: %v", err)
		}
	})
	if _, err := migrations.Up(ctx); err != nil {
		t.Fatal(err)
	}
	return pool
}
