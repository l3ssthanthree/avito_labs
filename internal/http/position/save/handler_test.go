package save_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
	"github.com/l3ssthanthree/avito_labs/internal/generated/api"
	transport "github.com/l3ssthanthree/avito_labs/internal/http"
	"github.com/l3ssthanthree/avito_labs/internal/http/position/save"
	"github.com/l3ssthanthree/avito_labs/internal/http/response"
)

func TestSavePositionHTTP(t *testing.T) {
	t.Parallel()
	const valid = `{"coordinates":{"latitude":0,"longitude":0},"recorded_at":"2026-11-20T12:00:00Z"}`
	for _, tc := range []struct {
		name, body string
		status     int
		usecaseErr error
	}{
		{"saved zeros", valid, 201, nil},
		{"oldest valid date", strings.ReplaceAll(valid, "2026-11-20T12:00:00Z", "0001-01-01T00:00:00Z"), 201, nil},
		{"missing", valid, 404, domain.ErrTripNotFound},
		{"completed", valid, 409, domain.ErrTripCompleted},
		{"future", valid, 400, &domain.ValidationError{Field: "recorded_at", Reason: "future"}},
		{"missing body", ``, 400, nil},
		{"missing date", `{"coordinates":{"latitude":0,"longitude":0}}`, 400, nil},
		{"null date", `{"coordinates":{"latitude":0,"longitude":0},"recorded_at":null}`, 400, nil},
		{"invalid date", strings.ReplaceAll(valid, "2026-11-20T12:00:00Z", "not-a-date"), 400, nil},
		{"absent latitude", strings.ReplaceAll(valid, `"latitude":0,`, ``), 400, nil},
		{"null latitude", strings.ReplaceAll(valid, `"latitude":0`, `"latitude":null`), 400, nil},
		{"latitude range", strings.ReplaceAll(valid, `"latitude":0`, `"latitude":91`), 400, nil},
		{"longitude range", strings.ReplaceAll(valid, `"longitude":0`, `"longitude":-181`), 400, nil},
		{"trailing object", valid + `{}`, 400, nil},
		{"unknown field", strings.ReplaceAll(valid, `"coordinates":`, `"extra":1,"coordinates":`), 400, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			uc := NewMockpositionUsecase(gomock.NewController(t))
			id := uuid.New()
			if tc.body == valid || tc.name == "oldest valid date" {
				var input api.PositionData
				if err := json.Unmarshal([]byte(tc.body), &input); err != nil {
					t.Fatal(err)
				}
				positionInput := domain.PositionInput{
					Coordinates: domain.Coordinates{Latitude: input.Coordinates.Latitude, Longitude: input.Coordinates.Longitude},
					RecordedAt:  input.RecordedAt,
				}
				uc.EXPECT().Save(gomock.Any(), id, positionInput).Return(domain.Position{ID: 42, TripID: id, PositionInput: positionInput}, tc.usecaseErr)
			}
			writer := response.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
			router, err := transport.NewRouter(transport.Endpoints{Save: save.New(uc, writer)}, writer)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/trips/"+id.String()+"/positions", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("got %d, want %d: %s", rec.Code, tc.status, rec.Body)
			}
			if tc.status == 201 {
				var saved api.TripPosition
				if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil || saved.Id != 42 || saved.TripId != id {
					t.Fatalf("bad position: %s", rec.Body)
				}
			}
		})
	}
}
