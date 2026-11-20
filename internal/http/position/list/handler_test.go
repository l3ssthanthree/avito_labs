package list_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
	"github.com/l3ssthanthree/avito_labs/internal/http/position/list"
	"github.com/l3ssthanthree/avito_labs/internal/http/response"
)

func TestListHTTP(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		points   []domain.Position
		failure  error
		status   int
		contains string
	}{
		{"empty", nil, nil, 200, `"positions":[]`},
		{"point", []domain.Position{{ID: 42, PositionInput: domain.PositionInput{RecordedAt: time.Now()}}}, nil, 200, `"id":42`},
		{"missing", nil, domain.ErrTripNotFound, 404, `"code":"trip_not_found"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			uc := NewMockpositionUsecase(gomock.NewController(t))
			id := uuid.New()
			uc.EXPECT().List(gomock.Any(), id).Return(tc.points, tc.failure)
			writer := response.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
			rec := httptest.NewRecorder()
			list.New(uc, writer).ListTripPositions(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil), id)
			if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.contains) {
				t.Fatalf("status %d body %s", rec.Code, rec.Body)
			}
		})
	}
}
