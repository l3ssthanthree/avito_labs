package create

import (
	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
	"github.com/l3ssthanthree/avito_labs/internal/generated/api"
)

func toInput(request api.CreateTripJSONRequestBody) domain.CreateInput {
	return domain.CreateInput{
		UserID: request.UserId, DriverID: request.DriverId,
		StartPoint: domain.Coordinates{Latitude: request.StartPoint.Latitude, Longitude: request.StartPoint.Longitude},
		EndPoint:   domain.Coordinates{Latitude: request.EndPoint.Latitude, Longitude: request.EndPoint.Longitude},
		Price:      request.Price,
	}
}
