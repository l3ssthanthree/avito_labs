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
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
	"github.com/l3ssthanthree/avito_labs/internal/infra/postgres/transaction"
	repository "github.com/l3ssthanthree/avito_labs/internal/repository/trip"
	"github.com/l3ssthanthree/avito_labs/internal/usecase/trip/finish"
)

func TestPositionMigrationsDownAndUp(t *testing.T) {
	t.Parallel()
	pool := isolatedPool(t)
	ctx, cancel := context.WithTimeout(t.Context(), databaseTimeout)
	defer cancel()
	provider, err := goose.NewProvider(goose.DialectPostgres, stdlib.OpenDBFromPool(pool), os.DirFS("../../../migrations"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := provider.Close(); err != nil {
			t.Error(err)
		}
	}()
	for range 2 {
		if _, err := provider.Down(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var exists bool
	err = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema() AND table_name = 'trips' AND column_name = 'last_position_at')`).Scan(&exists)
	if err != nil || exists {
		t.Fatalf("down migration left position column: %v %v", exists, err)
	}
	repo := repository.New(transaction.New(pool))
	legacy := newTrip(t)
	if err := repo.Create(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema() AND table_name = 'trips' AND column_name = 'last_position_at')`).Scan(&exists)
	if err != nil || !exists {
		t.Fatalf("up migration did not restore position column: %v %v", exists, err)
	}
	restored, err := repo.Get(ctx, legacy.ID)
	if err != nil || restored.ID != legacy.ID || restored.LastPositionAt != nil {
		t.Fatalf("migration changed legacy trip: %+v %v", restored, err)
	}
}

func TestStaleEligibility(t *testing.T) {
	t.Parallel()
	pool := isolatedPool(t)
	ctx, cancel := context.WithTimeout(t.Context(), databaseTimeout)
	defer cancel()
	repo := repository.New(transaction.New(pool))
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	old, fresh := now.Add(-2*time.Minute), now.Add(-30*time.Second)
	cases := []struct {
		name                string
		started             time.Time
		last                *time.Time
		completed, eligible bool
	}{
		{"no coordinates and old", old, nil, false, true},
		{"no coordinates and fresh", fresh, nil, false, false},
		{"old coordinates", old, &old, false, true},
		{"fresh coordinates override old start", old, &fresh, false, false},
		{"completed", old, &old, true, false},
	}
	wanted := make(map[uuid.UUID]bool)
	for _, tc := range cases {
		entity := newTrip(t)
		entity.StartedAt = tc.started
		if tc.completed {
			if err := entity.Finish(now); err != nil {
				t.Fatal(err)
			}
		}
		if err := repo.Create(ctx, entity); err != nil {
			t.Fatal(err)
		}
		if tc.last != nil {
			if _, err := pool.Exec(ctx, "UPDATE trips SET last_position_at = $2 WHERE id = $1", entity.ID, tc.last); err != nil {
				t.Fatal(err)
			}
		}
		needed, err := repo.NeedsPosition(ctx, entity.ID, now, time.Minute)
		if err != nil || needed != tc.eligible {
			t.Fatalf("%s: eligible %v, error %v", tc.name, needed, err)
		}
		if tc.eligible {
			wanted[entity.ID] = true
		}
	}
	trips, err := repo.FindStale(ctx, now, time.Minute, nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(trips) != len(wanted) {
		t.Fatalf("got %d stale trips, want %d", len(trips), len(wanted))
	}
	for _, trip := range trips {
		if !wanted[trip.ID] {
			t.Fatalf("fresh or completed trip selected: %v", trip.ID)
		}
		delete(wanted, trip.ID)
	}
	if len(wanted) != 0 {
		t.Fatal("stale trip omitted")
	}
}

func TestPositionAtomicityOrderAndCompleted(t *testing.T) {
	t.Parallel()
	pool := isolatedPool(t)
	ctx, cancel := context.WithTimeout(t.Context(), databaseTimeout)
	defer cancel()
	manager := transaction.New(pool)
	repo := repository.New(manager)
	entity := newTrip(t)
	if err := repo.Create(ctx, entity); err != nil {
		t.Fatal(err)
	}
	fresh := time.Now().UTC().Truncate(time.Microsecond)
	times := []time.Time{fresh, fresh.Add(-time.Hour), fresh}
	for _, at := range times {
		if err := manager.Do(ctx, func(txCtx context.Context) error {
			_, err := repo.SavePosition(txCtx, entity.ID, domain.PositionInput{RecordedAt: at})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.Get(ctx, entity.ID)
	if err != nil || got.LastPositionAt == nil || !got.LastPositionAt.Equal(fresh) {
		t.Fatalf("last position regressed: %+v %v", got, err)
	}
	points, err := repo.ListPositions(ctx, entity.ID)
	if err != nil || len(points) != 3 || !points[0].RecordedAt.Before(points[1].RecordedAt) || points[1].ID >= points[2].ID {
		t.Fatalf("route not sorted: %+v %v", points, err)
	}
	err = manager.Do(ctx, func(txCtx context.Context) error {
		_, err := repo.SavePosition(txCtx, entity.ID, domain.PositionInput{RecordedAt: fresh.Add(time.Minute), Coordinates: domain.Coordinates{Latitude: 100}})
		return err
	})
	if err == nil {
		t.Fatal("invalid coordinate accepted")
	}
	got, err = repo.Get(ctx, entity.ID)
	if err != nil || got.LastPositionAt == nil || !got.LastPositionAt.Equal(fresh) {
		t.Fatalf("timestamp survived failed insert: %+v %v", got, err)
	}
	points, err = repo.ListPositions(ctx, entity.ID)
	if err != nil || len(points) != 3 {
		t.Fatalf("failed insert affected route: %v %v", points, err)
	}
	finished, err := finish.New(repo, manager, time.Second).Finish(ctx, entity.ID)
	if err != nil || finished.LastPositionAt == nil || !finished.LastPositionAt.Equal(fresh) {
		t.Fatalf("finish lost position: %+v %v", finished, err)
	}
	err = manager.Do(ctx, func(txCtx context.Context) error {
		_, err := repo.SavePosition(txCtx, entity.ID, domain.PositionInput{RecordedAt: fresh})
		return err
	})
	if !errors.Is(err, domain.ErrTripCompleted) {
		t.Fatalf("completed trip: %v", err)
	}
	if _, err := repo.SavePosition(ctx, uuid.New(), domain.PositionInput{RecordedAt: fresh}); !errors.Is(err, domain.ErrTripNotFound) {
		t.Fatalf("missing trip: %v", err)
	}
}

func TestStaleSelectionBatchesAndIndex(t *testing.T) {
	t.Parallel()
	pool := isolatedPool(t)
	ctx, cancel := context.WithTimeout(t.Context(), databaseTimeout)
	defer cancel()
	repo := repository.New(transaction.New(pool))
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	_, err := pool.Exec(ctx, `INSERT INTO trips (id, user_id, driver_id, start_latitude, start_longitude,
		end_latitude, end_longitude, price, status, started_at, last_position_at, finished_at)
        SELECT md5('trip-' || i)::uuid, md5('user-' || i)::uuid, md5('driver-' || i)::uuid,
        0,0,0,0,0, CASE WHEN i > 4000 THEN 'completed' ELSE 'active' END,
        $1::timestamptz - interval '2 minutes',
		CASE WHEN i <= 250 THEN NULL ELSE $1::timestamptz END,
		CASE WHEN i > 4000 THEN $1::timestamptz ELSE NULL END FROM generate_series(1,5000) AS i`, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "ANALYZE trips"); err != nil {
		t.Fatal(err)
	}
	var cursor *domain.StaleTrip
	ids := make(map[uuid.UUID]bool)
	for batch := range 3 {
		rows, err := repo.FindStale(ctx, now, time.Minute, cursor, 100)
		want := 100
		if batch == 2 {
			want = 50
		}
		if err != nil || len(rows) != want {
			t.Fatalf("batch %d: %d rows, %v", batch, len(rows), err)
		}
		for _, row := range rows {
			if ids[row.ID] {
				t.Fatalf("repeated trip %v across equal-time batches", row.ID)
			}
			ids[row.ID] = true
		}
		last := rows[len(rows)-1]
		cursor = &last
	}
	rows, err := repo.FindStale(ctx, now, time.Minute, cursor, 100)
	if err != nil || len(rows) != 0 || len(ids) != 250 {
		t.Fatalf("end of selection: %v %v", rows, err)
	}
	for _, continuation := range []bool{false, true} {
		query := `EXPLAIN (ANALYZE, BUFFERS) SELECT id, driver_id, COALESCE(last_position_at, started_at)
            FROM trips WHERE status = 'active' AND COALESCE(last_position_at, started_at) < $1::timestamptz - $2 * interval '1 second'`
		args := []any{now, 60.0}
		if continuation {
			query += ` AND (COALESCE(last_position_at, started_at), id) > ($3::timestamptz, $4::uuid)`
			args = append(args, now.Add(-2*time.Minute), uuid.Nil)
		}
		query += ` ORDER BY COALESCE(last_position_at, started_at), id LIMIT 100`
		planRows, err := pool.Query(ctx, query, args...)
		if err != nil {
			t.Fatal(err)
		}
		var plan strings.Builder
		for planRows.Next() {
			var line string
			if err := planRows.Scan(&line); err != nil {
				t.Fatal(err)
			}
			plan.WriteString(line + "\n")
		}
		planRows.Close()
		if err := planRows.Err(); err != nil {
			t.Fatal(err)
		}
		t.Logf("continuation=%v\n%s", continuation, plan.String())
		if !strings.Contains(plan.String(), "trips_stale_positions_idx") || strings.Contains(plan.String(), "Seq Scan") {
			t.Fatalf("stale query does not use index:\n%s", plan.String())
		}
	}
	boundary := newTrip(t)
	boundary.StartedAt = now.Add(-time.Minute)
	if err := repo.Create(ctx, boundary); err != nil {
		t.Fatal(err)
	}
	needed, err := repo.NeedsPosition(ctx, boundary.ID, now, time.Minute)
	if err != nil || needed {
		t.Fatalf("exact stale boundary: needed %v, error %v", needed, err)
	}
	needed, err = repo.NeedsPosition(ctx, boundary.ID, now.Add(time.Microsecond), time.Minute)
	if err != nil || !needed {
		t.Fatalf("beyond boundary: needed %v, error %v", needed, err)
	}
}

func TestConcurrentPositionsAndFinish(t *testing.T) {
	t.Parallel()
	pool := isolatedPool(t)
	ctx, cancel := context.WithTimeout(t.Context(), databaseTimeout)
	defer cancel()
	manager := transaction.New(pool)
	repo := repository.New(manager)
	entity := newTrip(t)
	if err := repo.Create(ctx, entity); err != nil {
		t.Fatal(err)
	}
	fresh := time.Now().UTC().Truncate(time.Microsecond)
	results := make(chan error, 2)
	release := make(chan struct{})
	for _, at := range []time.Time{fresh, fresh.Add(-time.Hour)} {
		go func() {
			<-release
			results <- manager.Do(ctx, func(txCtx context.Context) error {
				_, err := repo.SavePosition(txCtx, entity.ID, domain.PositionInput{RecordedAt: at})
				return err
			})
		}()
	}
	close(release)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.Get(ctx, entity.ID)
	if err != nil || got.LastPositionAt == nil || !got.LastPositionAt.Equal(fresh) {
		t.Fatalf("concurrent old position regressed timestamp: %+v %v", got, err)
	}
	release = make(chan struct{})
	go func() {
		<-release
		_, err := finish.New(repo, manager, time.Second).Finish(ctx, entity.ID)
		results <- err
	}()
	go func() {
		<-release
		results <- manager.Do(ctx, func(txCtx context.Context) error {
			_, err := repo.SavePosition(txCtx, entity.ID, domain.PositionInput{RecordedAt: fresh})
			return err
		})
	}()
	close(release)
	for range 2 {
		if err := <-results; err != nil && !errors.Is(err, domain.ErrTripCompleted) {
			t.Fatal(err)
		}
	}
	needed, err := repo.NeedsPosition(ctx, entity.ID, fresh.Add(time.Hour), time.Minute)
	if err != nil || needed {
		t.Fatalf("finished trip still needs polling: %v %v", needed, err)
	}
}
