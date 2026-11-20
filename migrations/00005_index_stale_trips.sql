-- +goose NO TRANSACTION
-- +goose Up
CREATE INDEX CONCURRENTLY trips_stale_positions_idx
    ON trips (COALESCE(last_position_at, started_at), id) INCLUDE (driver_id)
    WHERE status = 'active';

-- +goose Down
DROP INDEX CONCURRENTLY trips_stale_positions_idx;
