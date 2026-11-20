package finish

import (
	"net/http"

	"github.com/l3ssthanthree/avito_labs/internal/generated/api"
	"github.com/l3ssthanthree/avito_labs/internal/http/response"
	tripresponse "github.com/l3ssthanthree/avito_labs/internal/http/trip"
)

type Finish struct {
	usecase  tripUsecase
	response *response.Writer
}

func New(usecase tripUsecase, writer *response.Writer) *Finish {
	return &Finish{usecase: usecase, response: writer}
}

func (h *Finish) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	trip, err := h.usecase.Finish(r.Context(), tripID)
	if err != nil {
		h.response.Error(w, r, err)
		return
	}
	h.response.JSON(w, r, http.StatusOK, tripresponse.ToAPITrip(trip))
}
