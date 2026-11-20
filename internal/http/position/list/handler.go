package list

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/l3ssthanthree/avito_labs/internal/generated/api"
	"github.com/l3ssthanthree/avito_labs/internal/http/position"
	"github.com/l3ssthanthree/avito_labs/internal/http/response"
)

type List struct {
	usecase  positionUsecase
	response *response.Writer
}

func New(usecase positionUsecase, writer *response.Writer) *List {
	return &List{usecase: usecase, response: writer}
}

func (h *List) ListTripPositions(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	points, err := h.usecase.List(r.Context(), id)
	if err != nil {
		h.response.Error(w, r, err)
		return
	}
	result := api.TripPositionsResponse{Positions: make([]api.TripPosition, 0, len(points))}
	for _, point := range points {
		result.Positions = append(result.Positions, position.ToAPI(point))
	}
	h.response.JSON(w, r, http.StatusOK, result)
}
