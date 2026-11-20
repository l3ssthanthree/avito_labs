//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}

package positions

import (
	"context"
	"time"

	"github.com/google/uuid"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

type tripRepository interface {
	FindStale(ctx context.Context, now time.Time, staleAfter time.Duration, after *domain.StaleTrip, limit int) ([]domain.StaleTrip, error)
	NeedsPosition(ctx context.Context, id uuid.UUID, now time.Time, staleAfter time.Duration) (bool, error)
}

type pushGateway interface {
	RequestPosition(ctx context.Context, tripID, driverID uuid.UUID) error
}

type Clock interface {
	Now() time.Time
	NewTicker(interval time.Duration) Ticker
}

type Ticker interface {
	C() <-chan time.Time
	Stop()
}
