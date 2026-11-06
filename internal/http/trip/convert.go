package trip

import (
	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
	"github.com/l3ssthanthree/avito_labs/internal/generated/api"
)

func ToAPITrip(trip domain.Trip) api.Trip {
	return api.Trip{
		Id: trip.ID, UserId: trip.UserID, DriverId: trip.DriverID,
		StartPoint: api.Coordinates{Latitude: trip.StartPoint.Latitude, Longitude: trip.StartPoint.Longitude},
		EndPoint:   api.Coordinates{Latitude: trip.EndPoint.Latitude, Longitude: trip.EndPoint.Longitude},
		Price:      trip.Price, Status: api.TripStatus(trip.Status()), StartedAt: trip.StartedAt,
		FinishedAt: trip.FinishedAt(), LastPositionAt: trip.LastPositionAt,
	}
}
