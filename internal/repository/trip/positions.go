package trip

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

func (r *Repository) SavePosition(ctx context.Context, id uuid.UUID, input domain.PositionInput) (domain.Position, error) {
	var position domain.Position
	err := r.database.QueryRow(ctx, `UPDATE trips
        SET last_position_at = GREATEST(last_position_at, $2::timestamptz), updated_at = now()
        WHERE id = $1 AND status = 'active' RETURNING id`, id, input.RecordedAt).Scan(&position.TripID)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, getErr := r.Get(ctx, id); getErr != nil {
			return position, getErr
		}
		return position, domain.ErrTripCompleted
	}
	if err != nil {
		return position, fmt.Errorf("update last position time: %w", err)
	}
	position.PositionInput = input
	err = r.database.QueryRow(ctx, `INSERT INTO trip_positions (trip_id, latitude, longitude, recorded_at)
        VALUES ($1, $2, $3, $4) RETURNING id, recorded_at`, id, input.Coordinates.Latitude, input.Coordinates.Longitude, input.RecordedAt).
		Scan(&position.ID, &position.RecordedAt)
	if err != nil {
		return domain.Position{}, fmt.Errorf("insert position: %w", err)
	}
	return position, nil
}

func (r *Repository) ListPositions(ctx context.Context, id uuid.UUID) ([]domain.Position, error) {
	rows, err := r.database.Query(ctx, `SELECT id, trip_id, latitude, longitude, recorded_at
        FROM trip_positions WHERE trip_id = $1 ORDER BY recorded_at, id`, id)
	if err != nil {
		return nil, fmt.Errorf("select positions: %w", err)
	}
	defer rows.Close()
	positions, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Position, error) {
		var p domain.Position
		err := row.Scan(&p.ID, &p.TripID, &p.Coordinates.Latitude, &p.Coordinates.Longitude, &p.RecordedAt)
		return p, err
	})
	if err != nil {
		return nil, fmt.Errorf("read positions: %w", err)
	}
	return positions, nil
}

func (r *Repository) FindStale(ctx context.Context, now time.Time, staleAfter time.Duration, after *domain.StaleTrip, limit int) ([]domain.StaleTrip, error) {
	query := `SELECT id, driver_id, COALESCE(last_position_at, started_at)
        FROM trips WHERE status = 'active'
        AND COALESCE(last_position_at, started_at) < $1::timestamptz - $2 * interval '1 second'`
	args := []any{now, staleAfter.Seconds()}
	if after != nil {
		query += ` AND (COALESCE(last_position_at, started_at), id) > ($3::timestamptz, $4::uuid)`
		args = append(args, after.LastSeen, after.ID)
	}
	query += fmt.Sprintf(" ORDER BY COALESCE(last_position_at, started_at), id LIMIT $%d", len(args)+1)
	args = append(args, limit)
	rows, err := r.database.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select stale trips: %w", err)
	}
	defer rows.Close()
	trips, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.StaleTrip, error) {
		var trip domain.StaleTrip
		err := row.Scan(&trip.ID, &trip.DriverID, &trip.LastSeen)
		return trip, err
	})
	if err != nil {
		return nil, fmt.Errorf("read stale trips: %w", err)
	}
	return trips, nil
}

func (r *Repository) NeedsPosition(ctx context.Context, id uuid.UUID, now time.Time, staleAfter time.Duration) (bool, error) {
	var needed bool
	err := r.database.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM trips WHERE id = $1 AND status = 'active'
        AND COALESCE(last_position_at, started_at) < $2::timestamptz - $3 * interval '1 second')`, id, now, staleAfter.Seconds()).Scan(&needed)
	if err != nil {
		return false, fmt.Errorf("check stale trip: %w", err)
	}
	return needed, nil
}
