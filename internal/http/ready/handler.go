package ready

import (
	"context"
	"net/http"
	"time"

	"github.com/l3ssthanthree/avito_labs/internal/generated/api"
	"github.com/l3ssthanthree/avito_labs/internal/http/response"
)

type Readiness struct {
	database     databasePinger
	queryTimeout time.Duration
	response     *response.Writer
}

func New(database databasePinger, queryTimeout time.Duration, writer *response.Writer) *Readiness {
	return &Readiness{database: database, queryTimeout: queryTimeout, response: writer}
}

func (h *Readiness) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()

	if err := h.database.Ping(ctx); err != nil {
		h.response.JSON(w, r, http.StatusServiceUnavailable, api.HealthResponse{Status: api.Unavailable})
		return
	}
	h.response.JSON(w, r, http.StatusOK, api.HealthResponse{Status: api.Ok})
}
