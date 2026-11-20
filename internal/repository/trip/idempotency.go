package trip

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/l3ssthanthree/avito_labs/internal/model"
)

func (r *Repository) DeleteExpiredIdempotencyKey(ctx context.Context, key uuid.UUID, now time.Time) error {
	query, args, err := r.builder.
		Delete("trip_create_idempotency").
		Where(sq.Eq{"idempotency_key": key}).
		Where(sq.LtOrEq{"expires_at": now}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build delete expired idempotency key query: %w", err)
	}

	if _, err := r.database.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("delete expired idempotency key: %w", err)
	}
	return nil
}

func (r *Repository) ReserveIdempotencyKey(
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
	err = r.database.QueryRow(ctx, query, args...).Scan(&reservedKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reserve idempotency key: %w", err)
	}
	return true, nil
}

func (r *Repository) GetIdempotencyRecord(ctx context.Context, key uuid.UUID) (model.IdempotencyRecord, error) {
	query, args, err := r.builder.
		Select("request_hash", "trip_id").
		From("trip_create_idempotency").
		Where(sq.Eq{"idempotency_key": key}).
		ToSql()
	if err != nil {
		return model.IdempotencyRecord{}, fmt.Errorf("build get idempotency record query: %w", err)
	}

	var record model.IdempotencyRecord
	if err := r.database.QueryRow(ctx, query, args...).Scan(&record.RequestHash, &record.TripID); err != nil {
		return model.IdempotencyRecord{}, fmt.Errorf("select idempotency record: %w", err)
	}
	return record, nil
}
