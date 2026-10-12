-- +goose Up
CREATE TABLE trip_create_idempotency (
    idempotency_key UUID PRIMARY KEY,
    request_hash    BYTEA NOT NULL CHECK (octet_length(request_hash) = 32),
    trip_id         UUID NOT NULL REFERENCES trips(id) DEFERRABLE INITIALLY DEFERRED,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL,
    CHECK (expires_at > created_at)
);

CREATE INDEX trip_create_idempotency_expires_at_idx
    ON trip_create_idempotency (expires_at);

-- +goose Down
DROP TABLE IF EXISTS trip_create_idempotency;
