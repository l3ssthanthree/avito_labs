package get

import (
	"net/http"

	"github.com/l3ssthanthree/avito_labs/internal/generated/api"
	"github.com/l3ssthanthree/avito_labs/internal/http/response"
	tripresponse "github.com/l3ssthanthree/avito_labs/internal/http/trip"
)

type Get struct {
	usecase  tripUsecase
	response *response.Writer
}

func New(usecase tripUsecase, writer *response.Writer) *Get {
	return &Get{usecase: usecase, response: writer}
}

func (h *Get) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	trip, err := h.usecase.Get(r.Context(), tripID)
	if err != nil {
		h.response.Error(w, r, err)
		return
	}
	h.response.JSON(w, r, http.StatusOK, tripresponse.ToAPITrip(trip))
}
