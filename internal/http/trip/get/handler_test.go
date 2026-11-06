package get_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
	"github.com/l3ssthanthree/avito_labs/internal/generated/api"
	"github.com/l3ssthanthree/avito_labs/internal/http/response"
	"github.com/l3ssthanthree/avito_labs/internal/http/trip/get"
)

func TestGetMissingTrip(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	uc := NewMocktripUsecase(gomock.NewController(t))
	uc.EXPECT().Get(gomock.Any(), id).Return(domain.Trip{}, domain.ErrTripNotFound)
	writer := response.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/trips/"+id.String(), nil)
	rec := httptest.NewRecorder()
	get.New(uc, writer).GetTrip(rec, req, id)

	var problem api.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil || rec.Code != http.StatusNotFound || problem.Code != "trip_not_found" {
		t.Fatalf("got HTTP %d, %s", rec.Code, rec.Body)
	}
}
