package position

import (
	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
	"github.com/l3ssthanthree/avito_labs/internal/generated/api"
)

func ToAPI(p domain.Position) api.TripPosition {
	return api.TripPosition{
		Id: p.ID, TripId: p.TripID, RecordedAt: p.RecordedAt,
		Coordinates: api.Coordinates{Latitude: p.Coordinates.Latitude, Longitude: p.Coordinates.Longitude},
	}
}
