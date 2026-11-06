package trip

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

func (r *Repository) CountActive(ctx context.Context) (int64, error) {
	var count int64
	err := r.database.QueryRow(ctx, "SELECT count(*) FROM trips WHERE status = $1", domain.StatusActive).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count active trips: %w", err)
	}
	return count, nil
}

func (r *Repository) Get(ctx context.Context, tripID uuid.UUID) (domain.Trip, error) {
	query, args, err := r.builder.
		Select(
			"id", "user_id", "driver_id",
			"start_latitude", "start_longitude",
			"end_latitude", "end_longitude",
			"price", "status", "started_at", "finished_at",
		).
		From("trips").
		Where(sq.Eq{"id": tripID}).
		ToSql()
	if err != nil {
		return domain.Trip{}, fmt.Errorf("build get trip query: %w", err)
	}

	trip, err := scanTrip(r.database.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Trip{}, domain.ErrTripNotFound
	}
	if err != nil {
		return domain.Trip{}, fmt.Errorf("select trip: %w", err)
	}
	return trip, nil
}
