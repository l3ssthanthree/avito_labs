package domainerror

import (
	"errors"
	"net/http"
	"strings"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
	"github.com/l3ssthanthree/avito_labs/internal/generated/api"
)

func Problem(err error) (int, api.Problem) {
	var validation *domain.ValidationError
	switch {
	case errors.As(err, &validation):
		return InvalidRequest(validation.Error())
	case errors.Is(err, domain.ErrTripNotFound):
		return NewProblem(http.StatusNotFound, "trip_not_found", "Trip not found", "Trip does not exist")
	case errors.Is(err, domain.ErrTripCompleted):
		return NewProblem(http.StatusConflict, "trip_completed", "Trip completed", "Trip is already completed")
	case errors.Is(err, domain.ErrDriverBusy):
		return NewProblem(http.StatusConflict, "driver_busy", "Driver busy", "Driver already has an active trip")
	case errors.Is(err, domain.ErrIdempotencyConflict):
		return NewProblem(http.StatusConflict, "idempotency_conflict", "Idempotency conflict", "Idempotency-Key was already used with a different request body")
	default:
		return NewProblem(http.StatusInternalServerError, "internal_error", "Internal error", "Internal server error")
	}
}

func InvalidRequest(detail string) (int, api.Problem) {
	return NewProblem(http.StatusBadRequest, "invalid_request", "Invalid request", detail)
}

func NewProblem(status int32, code, title, detail string) (int, api.Problem) {
	return int(status), api.Problem{
		Type:  "https://tripgo.example/problems/" + strings.ReplaceAll(code, "_", "-"),
		Title: title, Status: status, Detail: &detail, Code: code,
	}
}
