package create

import (
	"net/http"

	"github.com/l3ssthanthree/avito_labs/internal/generated/api"
	"github.com/l3ssthanthree/avito_labs/internal/http/response"
	tripresponse "github.com/l3ssthanthree/avito_labs/internal/http/trip"
)

type Create struct {
	usecase  tripUsecase
	response *response.Writer
}

func New(usecase tripUsecase, writer *response.Writer) *Create {
	return &Create{usecase: usecase, response: writer}
}

func (h *Create) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	var request api.CreateTripJSONRequestBody
	if err := response.DecodeJSON(r.Body, &request); err != nil {
		h.response.RequestError(w, r, err)
		return
	}

	trip, replayed, err := h.usecase.Create(r.Context(), toInput(request), params.IdempotencyKey)
	if err != nil {
		h.response.Error(w, r, err)
		return
	}

	w.Header().Set("Location", "/api/v1/trips/"+trip.ID.String())
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	h.response.JSON(w, r, status, tripresponse.ToAPITrip(trip))
}
