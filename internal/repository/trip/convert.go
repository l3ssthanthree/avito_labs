package trip

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

func scanTrip(row pgx.Row) (domain.Trip, error) {
	var (
		id         uuid.UUID
		input      domain.CreateInput
		status     domain.Status
		startedAt  time.Time
		finishedAt *time.Time
	)
	if err := row.Scan(
		&id, &input.UserID, &input.DriverID,
		&input.StartPoint.Latitude, &input.StartPoint.Longitude,
		&input.EndPoint.Latitude, &input.EndPoint.Longitude,
		&input.Price, &status, &startedAt, &finishedAt,
	); err != nil {
		return domain.Trip{}, err
	}
	return domain.Restore(id, input, startedAt, status, finishedAt), nil
}
