package domainerror_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
	"github.com/l3ssthanthree/avito_labs/internal/http/domainerror"
)

func TestProblem(t *testing.T) {
	t.Parallel()
	const secret = "private database address"
	for _, tc := range []struct {
		name, code, detail string
		err                error
		status             int
	}{
		{"invalid", "invalid_request", "price: must not be negative", &domain.ValidationError{Field: "price", Reason: "must not be negative"}, http.StatusBadRequest},
		{"missing", "trip_not_found", "", domain.ErrTripNotFound, http.StatusNotFound},
		{"completed", "trip_completed", "", domain.ErrTripCompleted, http.StatusConflict},
		{"busy", "driver_busy", "", domain.ErrDriverBusy, http.StatusConflict},
		{"idempotency", "idempotency_conflict", "", domain.ErrIdempotencyConflict, http.StatusConflict},
		{"internal", "internal_error", "Internal server error", errors.New(secret), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			status, problem := domainerror.Problem(fmt.Errorf("%s: %w", secret, tc.err))
			if status != tc.status || int(problem.Status) != status || problem.Code != tc.code {
				t.Fatalf("got HTTP %d, problem %+v, want %d / %s", status, problem, tc.status, tc.code)
			}
			if problem.Detail == nil || strings.Contains(*problem.Detail, secret) || (tc.detail != "" && *problem.Detail != tc.detail) {
				t.Fatalf("unexpected detail: %v", problem.Detail)
			}
		})
	}
}
