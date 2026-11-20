package health

import (
	"net/http"

	"github.com/l3ssthanthree/avito_labs/internal/generated/api"
	"github.com/l3ssthanthree/avito_labs/internal/http/response"
)

type Liveness struct {
	response *response.Writer
}

func New(writer *response.Writer) *Liveness {
	return &Liveness{response: writer}
}

func (h *Liveness) Health(w http.ResponseWriter, r *http.Request) {
	h.response.JSON(w, r, http.StatusOK, api.HealthResponse{Status: api.Ok})
}
