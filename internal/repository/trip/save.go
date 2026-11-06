package trip

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

const activeDriverConstraint = "trips_one_active_per_driver_idx"

func (r *Repository) Create(ctx context.Context, trip domain.Trip) error {
	query, args, err := r.builder.
		Insert("trips").
		Columns(
			"id", "user_id", "driver_id",
			"start_latitude", "start_longitude",
			"end_latitude", "end_longitude",
			"price", "status", "started_at", "finished_at",
		).
		Values(
			trip.ID, trip.UserID, trip.DriverID,
			trip.StartPoint.Latitude, trip.StartPoint.Longitude,
			trip.EndPoint.Latitude, trip.EndPoint.Longitude,
			trip.Price, trip.Status(), trip.StartedAt, trip.FinishedAt(),
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("build create trip query: %w", err)
	}

	_, err = r.database.Exec(ctx, query, args...)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" && postgresError.ConstraintName == activeDriverConstraint {
			return domain.ErrDriverBusy
		}
		return fmt.Errorf("insert trip: %w", err)
	}
	return nil
}

func (r *Repository) AddStatusHistory(ctx context.Context, tripID uuid.UUID, fromStatus *domain.Status, toStatus domain.Status, reason *string) error {
	query, args, err := r.builder.
		Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason").
		Values(tripID, fromStatus, toStatus, reason).
		ToSql()
	if err != nil {
		return fmt.Errorf("build status history query: %w", err)
	}

	if _, err := r.database.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert trip status history: %w", err)
	}
	return nil
}

func (r *Repository) SaveFinished(ctx context.Context, finished domain.Trip) (domain.Trip, error) {
	finishedAt := finished.FinishedAt()
	query, args, err := r.builder.
		Update("trips").
		Set("status", finished.Status()).
		Set("finished_at", finishedAt).
		Set("updated_at", finishedAt).
		Where(sq.Eq{"id": finished.ID, "status": domain.StatusActive}).
		Suffix(`RETURNING id, user_id, driver_id,
			start_latitude, start_longitude,
			end_latitude, end_longitude,
			price, status, started_at, finished_at`).
		ToSql()
	if err != nil {
		return domain.Trip{}, fmt.Errorf("build finish trip query: %w", err)
	}

	trip, err := scanTrip(r.database.QueryRow(ctx, query, args...))
	if err == nil {
		return trip, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Trip{}, fmt.Errorf("update trip: %w", err)
	}

	existingTrip, err := r.Get(ctx, finished.ID)
	if err != nil {
		return domain.Trip{}, err
	}
	if existingTrip.Status() == domain.StatusCompleted {
		return domain.Trip{}, domain.ErrTripCompleted
	}
	return domain.Trip{}, fmt.Errorf("finish trip in unexpected status %q", existingTrip.Status())
}
