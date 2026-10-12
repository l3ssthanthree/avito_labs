package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/l3ssthanthree/avito_labs/internal/model"
	"github.com/l3ssthanthree/avito_labs/internal/txmanager"
)

const activeDriverConstraint = "trips_one_active_per_driver_idx"

type TripRepository struct {
	pool    *pgxpool.Pool
	builder sq.StatementBuilderType
}

func NewTripRepository(pool *pgxpool.Pool) *TripRepository {
	return &TripRepository{
		pool:    pool,
		builder: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
	}
}

func (r *TripRepository) Create(ctx context.Context, trip model.Trip) error {
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
			trip.Price, trip.Status, trip.StartedAt, trip.FinishedAt,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("build create trip query: %w", err)
	}

	_, err = txmanager.Executor(ctx, r.pool).Exec(ctx, query, args...)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" && postgresError.ConstraintName == activeDriverConstraint {
			return model.ErrDriverBusy
		}
		return fmt.Errorf("insert trip: %w", err)
	}
	return nil
}

func (r *TripRepository) DeleteExpiredIdempotencyKey(ctx context.Context, key uuid.UUID, now time.Time) error {
	query, args, err := r.builder.
		Delete("trip_create_idempotency").
		Where(sq.Eq{"idempotency_key": key}).
		Where(sq.LtOrEq{"expires_at": now}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build delete expired idempotency key query: %w", err)
	}

	if _, err := txmanager.Executor(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("delete expired idempotency key: %w", err)
	}
	return nil
}

func (r *TripRepository) ReserveIdempotencyKey(
	ctx context.Context,
	key uuid.UUID,
	requestHash []byte,
	tripID uuid.UUID,
	expiresAt time.Time,
) (bool, error) {
	query, args, err := r.builder.
		Insert("trip_create_idempotency").
		Columns("idempotency_key", "request_hash", "trip_id", "expires_at").
		Values(key, requestHash, tripID, expiresAt).
		Suffix("ON CONFLICT (idempotency_key) DO NOTHING RETURNING idempotency_key").
		ToSql()
	if err != nil {
		return false, fmt.Errorf("build reserve idempotency key query: %w", err)
	}

	var reservedKey uuid.UUID
	err = txmanager.Executor(ctx, r.pool).QueryRow(ctx, query, args...).Scan(&reservedKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reserve idempotency key: %w", err)
	}
	return true, nil
}

func (r *TripRepository) GetIdempotencyRecord(ctx context.Context, key uuid.UUID) (model.IdempotencyRecord, error) {
	query, args, err := r.builder.
		Select("request_hash", "trip_id").
		From("trip_create_idempotency").
		Where(sq.Eq{"idempotency_key": key}).
		ToSql()
	if err != nil {
		return model.IdempotencyRecord{}, fmt.Errorf("build get idempotency record query: %w", err)
	}

	var record model.IdempotencyRecord
	if err := txmanager.Executor(ctx, r.pool).QueryRow(ctx, query, args...).Scan(&record.RequestHash, &record.TripID); err != nil {
		return model.IdempotencyRecord{}, fmt.Errorf("select idempotency record: %w", err)
	}
	return record, nil
}

func (r *TripRepository) AddStatusHistory(ctx context.Context, tripID uuid.UUID, fromStatus *model.TripStatus, toStatus model.TripStatus, reason *string) error {
	query, args, err := r.builder.
		Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason").
		Values(tripID, fromStatus, toStatus, reason).
		ToSql()
	if err != nil {
		return fmt.Errorf("build status history query: %w", err)
	}

	if _, err := txmanager.Executor(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert trip status history: %w", err)
	}
	return nil
}

func (r *TripRepository) Get(ctx context.Context, tripID uuid.UUID) (model.Trip, error) {
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
		return model.Trip{}, fmt.Errorf("build get trip query: %w", err)
	}

	trip, err := scanTrip(txmanager.Executor(ctx, r.pool).QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Trip{}, model.ErrTripNotFound
	}
	if err != nil {
		return model.Trip{}, fmt.Errorf("select trip: %w", err)
	}
	return trip, nil
}

func (r *TripRepository) Finish(ctx context.Context, tripID uuid.UUID, finishedAt time.Time) (model.Trip, error) {
	query, args, err := r.builder.
		Update("trips").
		Set("status", model.TripStatusCompleted).
		Set("finished_at", finishedAt).
		Set("updated_at", finishedAt).
		Where(sq.Eq{"id": tripID, "status": model.TripStatusActive}).
		Suffix(`RETURNING id, user_id, driver_id,
			start_latitude, start_longitude,
			end_latitude, end_longitude,
			price, status, started_at, finished_at`).
		ToSql()
	if err != nil {
		return model.Trip{}, fmt.Errorf("build finish trip query: %w", err)
	}

	trip, err := scanTrip(txmanager.Executor(ctx, r.pool).QueryRow(ctx, query, args...))
	if err == nil {
		return trip, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.Trip{}, fmt.Errorf("update trip: %w", err)
	}

	existingTrip, err := r.Get(ctx, tripID)
	if err != nil {
		return model.Trip{}, err
	}
	if existingTrip.Status == model.TripStatusCompleted {
		return model.Trip{}, model.ErrTripCompleted
	}
	return model.Trip{}, fmt.Errorf("finish trip in unexpected status %q", existingTrip.Status)
}

func scanTrip(row pgx.Row) (model.Trip, error) {
	var trip model.Trip
	err := row.Scan(
		&trip.ID, &trip.UserID, &trip.DriverID,
		&trip.StartPoint.Latitude, &trip.StartPoint.Longitude,
		&trip.EndPoint.Latitude, &trip.EndPoint.Longitude,
		&trip.Price, &trip.Status, &trip.StartedAt, &trip.FinishedAt,
	)
	return trip, err
}
