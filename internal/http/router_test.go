package http_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/l3ssthanthree/avito_labs/internal/generated/api"
	transport "github.com/l3ssthanthree/avito_labs/internal/http"
	"github.com/l3ssthanthree/avito_labs/internal/http/health"
	"github.com/l3ssthanthree/avito_labs/internal/http/response"
)

func TestRoutingErrors(t *testing.T) {
	t.Parallel()
	const validTrip = "/api/v1/trips/4d5ff99e-1430-433f-bf05-1282ab5d79f9"
	writer := response.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	router, err := transport.NewRouter(transport.Endpoints{Liveness: health.New(writer)}, writer)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name, method, path, code, allow string
		status                          int
	}{
		{"unknown path", http.MethodGet, "/unknown", "route_not_found", "", http.StatusNotFound},
		{"unimplemented future route", http.MethodPost, validTrip + "/positions", "route_not_found", "", http.StatusNotFound},
		{"unknown method and path", "BREW", "/unknown", "route_not_found", "", http.StatusNotFound},
		{"wrong create method", http.MethodGet, "/api/v1/trips", "method_not_allowed", http.MethodPost, http.StatusMethodNotAllowed},
		{"wrong get method", http.MethodDelete, validTrip, "method_not_allowed", http.MethodGet, http.StatusMethodNotAllowed},
		{"wrong finish method", http.MethodGet, validTrip + "/finish", "method_not_allowed", http.MethodPost, http.StatusMethodNotAllowed},
		{"unknown method on known path", "BREW", "/health", "method_not_allowed", http.MethodGet, http.StatusMethodNotAllowed},
		{"invalid get UUID", http.MethodGet, "/api/v1/trips/bad-uuid", "invalid_request", "", http.StatusBadRequest},
		{"invalid finish UUID", http.MethodPost, "/api/v1/trips/bad-uuid/finish", "invalid_request", "", http.StatusBadRequest},
		{"health", http.MethodGet, "/health", "", "", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequestWithContext(t.Context(), tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.status || rec.Header().Get("Allow") != tc.allow {
				t.Fatalf("got status %d, Allow %q, body %s", rec.Code, rec.Header().Get("Allow"), rec.Body)
			}
			if tc.code != "" {
				var problem api.Problem
				if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil || problem.Code != tc.code ||
					int(problem.Status) != tc.status || rec.Header().Get("Content-Type") != "application/problem+json" {
					t.Fatalf("unexpected problem: %s", rec.Body)
				}
			}
		})
	}
}
