package save

import (
	"net/http"

	"github.com/google/uuid"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
	"github.com/l3ssthanthree/avito_labs/internal/generated/api"
	"github.com/l3ssthanthree/avito_labs/internal/http/position"
	"github.com/l3ssthanthree/avito_labs/internal/http/response"
)

type Save struct {
	usecase  positionUsecase
	response *response.Writer
}

func New(usecase positionUsecase, writer *response.Writer) *Save {
	return &Save{usecase: usecase, response: writer}
}

func (h *Save) CreateTripPosition(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var request api.CreateTripPositionJSONRequestBody
	if err := response.DecodeJSON(r.Body, &request); err != nil {
		h.response.RequestError(w, r, err)
		return
	}
	saved, err := h.usecase.Save(r.Context(), id, domain.PositionInput{
		Coordinates: domain.Coordinates{Latitude: request.Coordinates.Latitude, Longitude: request.Coordinates.Longitude},
		RecordedAt:  request.RecordedAt,
	})
	if err != nil {
		h.response.Error(w, r, err)
		return
	}
	h.response.JSON(w, r, http.StatusCreated, position.ToAPI(saved))
}
