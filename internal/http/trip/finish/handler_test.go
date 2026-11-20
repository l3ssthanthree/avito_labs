package finish_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
	"github.com/l3ssthanthree/avito_labs/internal/generated/api"
	"github.com/l3ssthanthree/avito_labs/internal/http/response"
	"github.com/l3ssthanthree/avito_labs/internal/http/trip/finish"
)

func TestFinishTrip(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	now := time.Now().UTC()
	entity := domain.Restore(id, domain.CreateInput{}, now.Add(-time.Minute), domain.StatusCompleted, &now)
	uc := NewMocktripUsecase(gomock.NewController(t))
	gomock.InOrder(
		uc.EXPECT().Finish(gomock.Any(), id).Return(entity, nil),
		uc.EXPECT().Finish(gomock.Any(), id).Return(domain.Trip{}, domain.ErrTripCompleted),
	)
	writer := response.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	handler := finish.New(uc, writer)
	path := "/api/v1/trips/" + id.String() + "/finish"

	rec := httptest.NewRecorder()
	handler.FinishTrip(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, nil), id)
	var got api.Trip
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK ||
		got.Id != id || got.Status != api.Completed || got.FinishedAt == nil || !got.FinishedAt.Equal(now) {
		t.Fatalf("got HTTP %d, %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	handler.FinishTrip(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, nil), id)
	var problem api.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil || rec.Code != http.StatusConflict || problem.Code != "trip_completed" {
		t.Fatalf("got HTTP %d, %s", rec.Code, rec.Body)
	}
}
