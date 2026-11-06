package create_test

import (
	"encoding/json"
	"errors"
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
	"github.com/l3ssthanthree/avito_labs/internal/generated/api"
	transport "github.com/l3ssthanthree/avito_labs/internal/http"
	"github.com/l3ssthanthree/avito_labs/internal/http/response"
	"github.com/l3ssthanthree/avito_labs/internal/http/trip/create"
)

func TestCreateTrip(t *testing.T) {
	t.Parallel()
	const privateDetail = "dial failed at private database address"
	const validBody = `{"user_id":"4d5ff99e-1430-433f-bf05-1282ab5d79f9","driver_id":"97d00811-3fa4-465b-80be-c74524355f77","start_point":{"latitude":0,"longitude":0},"end_point":{"latitude":0,"longitude":0},"price":0}`
	const path = "/api/v1/trips"

	for _, tc := range []struct {
		name, body, code string
		usecaseErr       error
		status           int
		replayed         bool
		keyHeaders       []string
	}{
		{name: "created", body: validBody, status: http.StatusCreated},
		{name: "replayed", body: validBody, status: http.StatusOK, replayed: true},
		{name: "busy", body: validBody, status: http.StatusConflict, code: "driver_busy", usecaseErr: domain.ErrDriverBusy},
		{name: "internal", body: validBody, status: http.StatusInternalServerError, code: "internal_error", usecaseErr: errors.New(privateDetail)},
		{name: "missing price", body: strings.Replace(validBody, `,"price":0`, "", 1), status: http.StatusBadRequest, code: "invalid_request"},
		{name: "null price", body: strings.Replace(validBody, `"price":0`, `"price":null`, 1), status: http.StatusBadRequest, code: "invalid_request"},
		{name: "missing latitude", body: strings.Replace(validBody, `"latitude":0,`, "", 1), status: http.StatusBadRequest, code: "invalid_request"},
		{name: "unknown field", body: strings.Replace(validBody, `"price":0`, `"price":0,"other":true`, 1), status: http.StatusBadRequest, code: "invalid_request"},
		{name: "trailing object", body: validBody + "{}", status: http.StatusBadRequest, code: "invalid_request"},
		{name: "malformed JSON", body: "{", status: http.StatusBadRequest, code: "invalid_request"},
		{name: "invalid idempotency key", body: validBody, status: http.StatusBadRequest, code: "invalid_request", keyHeaders: []string{"invalid"}},
		{
			name: "duplicate idempotency key", body: validBody, status: http.StatusBadRequest, code: "invalid_request",
			keyHeaders: []string{"4d5ff99e-1430-433f-bf05-1282ab5d79f9", "97d00811-3fa4-465b-80be-c74524355f77"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			id := uuid.New()
			key := uuid.New()
			uc := NewMocktripUsecase(gomock.NewController(t))
			if tc.status != http.StatusBadRequest {
				input := gomock.Cond(func(in domain.CreateInput) bool {
					return in.UserID.String() == "4d5ff99e-1430-433f-bf05-1282ab5d79f9" &&
						in.DriverID.String() == "97d00811-3fa4-465b-80be-c74524355f77" && in.Price == 0
				})
				entity := domain.Restore(id, domain.CreateInput{}, time.Now(), domain.StatusActive, nil)
				uc.EXPECT().Create(gomock.Any(), input, &key).Return(entity, tc.replayed, tc.usecaseErr)
			}
			writer := response.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
			router, err := transport.NewRouter(transport.Endpoints{Create: create.New(uc, writer)}, writer)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			headers := tc.keyHeaders
			if len(headers) == 0 {
				headers = []string{key.String()}
			}
			for _, header := range headers {
				req.Header.Add("Idempotency-Key", header)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.status {
				t.Fatalf("HTTP %d, want %d. Body: %s", rec.Code, tc.status, rec.Body)
			}
			if tc.code == "" {
				if rec.Header().Get("Location") != path+"/"+id.String() {
					t.Fatalf("unexpected Location: %q", rec.Header().Get("Location"))
				}
				var entity api.Trip
				if err := json.Unmarshal(rec.Body.Bytes(), &entity); err != nil || entity.Id != id {
					t.Fatalf("unexpected trip response: %s", rec.Body)
				}
				return
			}
			var problem api.Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil || problem.Code != tc.code ||
				rec.Header().Get("Content-Type") != "application/problem+json" || strings.Contains(rec.Body.String(), privateDetail) {
				t.Fatalf("unexpected problem response: %s", rec.Body)
			}
		})
	}
}

func TestCreateBodyLimit(t *testing.T) {
	t.Parallel()
	const limit = 1 << 20
	const validBody = `{"user_id":"4d5ff99e-1430-433f-bf05-1282ab5d79f9","driver_id":"97d00811-3fa4-465b-80be-c74524355f77","start_point":{"latitude":0,"longitude":0},"end_point":{"latitude":0,"longitude":0},"price":0}`
	for _, tc := range []struct {
		name          string
		size          int
		contentLength int64
		wantRead      int
		status        int
	}{
		{"exact limit", limit, limit, limit, http.StatusCreated},
		{"known size over limit", limit + 4096, limit + 4096, 0, http.StatusRequestEntityTooLarge},
		{"stream over limit", limit + 4096, -1, limit + 1, http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := &countedReader{Reader: strings.NewReader(validBody + strings.Repeat(" ", tc.size-len(validBody)))}
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/trips", body)
			req.ContentLength = tc.contentLength
			req.Header.Set("Content-Type", "application/json")
			uc := NewMocktripUsecase(gomock.NewController(t))
			if tc.status == http.StatusCreated {
				entity := domain.Restore(uuid.New(), domain.CreateInput{}, time.Now(), domain.StatusActive, nil)
				uc.EXPECT().Create(gomock.Any(), gomock.Any(), nil).Return(entity, false, nil)
			}
			writer := response.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
			router, err := transport.NewRouter(transport.Endpoints{Create: create.New(uc, writer)}, writer)
			if err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.status || body.read != tc.wantRead {
				t.Fatalf("HTTP %d, read %d bytes, want %d and %d. Body: %s", rec.Code, body.read, tc.status, tc.wantRead, rec.Body)
			}
			if tc.status == http.StatusRequestEntityTooLarge {
				var problem api.Problem
				if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil || problem.Code != "payload_too_large" {
					t.Fatalf("unexpected problem: %s", rec.Body)
				}
			}
		})
	}
}

type countedReader struct {
	io.Reader
	read int
}

func (r *countedReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.read += n
	return n, err
}
