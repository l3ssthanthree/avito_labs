package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/l3ssthanthree/avito_labs/api"
	"github.com/l3ssthanthree/avito_labs/internal/model"
	"github.com/l3ssthanthree/avito_labs/internal/service"
)

const maxRequestBodySize = 1 << 20

type Handler struct {
	database     *pgxpool.Pool
	tripService  *service.TripService
	queryTimeout time.Duration
	logger       *slog.Logger
}

func New(database *pgxpool.Pool, tripService *service.TripService, queryTimeout time.Duration, logger *slog.Logger) *Handler {
	return &Handler{
		database: database, tripService: tripService,
		queryTimeout: queryTimeout, logger: logger,
	}
}

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	var request api.CreateTripJSONRequestBody
	if err := decodeJSON(w, r, &request); err != nil {
		h.writeProblem(w, r, http.StatusBadRequest, "invalid_request", "Invalid request", err.Error())
		return
	}

	trip, replayed, err := h.tripService.Create(r.Context(), model.CreateTripInput{
		UserID: request.UserId, DriverID: request.DriverId,
		StartPoint: model.Coordinates{Latitude: request.StartPoint.Latitude, Longitude: request.StartPoint.Longitude},
		EndPoint:   model.Coordinates{Latitude: request.EndPoint.Latitude, Longitude: request.EndPoint.Longitude},
		Price:      request.Price,
	}, params.IdempotencyKey)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	w.Header().Set("Location", "/api/v1/trips/"+trip.ID.String())
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	h.writeJSON(w, status, toAPITrip(trip))
}

func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	trip, err := h.tripService.Get(r.Context(), uuid.UUID(tripID))
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, toAPITrip(trip))
}

func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	trip, err := h.tripService.Finish(r.Context(), uuid.UUID(tripID))
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, toAPITrip(trip))
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	h.writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()

	if err := h.database.Ping(ctx); err != nil {
		h.writeJSON(w, http.StatusServiceUnavailable, api.HealthResponse{Status: api.Unavailable})
		return
	}
	h.writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func (h *Handler) OpenAPIError(w http.ResponseWriter, r *http.Request, err error) {
	h.writeProblem(w, r, http.StatusBadRequest, "invalid_request", "Invalid request", err.Error())
}

func (h *Handler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, model.ErrInvalidRequest):
		h.writeProblem(w, r, http.StatusBadRequest, "invalid_request", "Invalid request", err.Error())
	case errors.Is(err, model.ErrTripNotFound):
		h.writeProblem(w, r, http.StatusNotFound, "trip_not_found", "Trip not found", "Trip does not exist")
	case errors.Is(err, model.ErrTripCompleted):
		h.writeProblem(w, r, http.StatusConflict, "trip_completed", "Trip completed", "Trip is already completed")
	case errors.Is(err, model.ErrDriverBusy):
		h.writeProblem(w, r, http.StatusConflict, "driver_busy", "Driver busy", "Driver already has an active trip")
	case errors.Is(err, model.ErrIdempotencyConflict):
		h.writeProblem(w, r, http.StatusConflict, "idempotency_conflict", "Idempotency conflict", "Idempotency-Key was already used with a different request body")
	default:
		h.logger.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
		h.writeProblem(w, r, http.StatusInternalServerError, "internal_error", "Internal error", "Internal server error")
	}
}

func (h *Handler) writeProblem(w http.ResponseWriter, r *http.Request, status int, code, title, detail string) {
	problemType := "https://tripgo.example/problems/" + problemSlug(code)
	instance := r.URL.Path
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(api.Problem{
		Type: problemType, Title: title, Status: int32(status),
		Detail: &detail, Instance: &instance, Code: code,
	}); err != nil {
		h.logger.Error("encode problem response", "error", err)
	}
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		h.logger.Error("encode JSON response", "error", err)
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode request body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("request body must contain a single JSON object")
	}
	return nil
}

func toAPITrip(trip model.Trip) api.Trip {
	return api.Trip{
		Id: trip.ID, UserId: trip.UserID, DriverId: trip.DriverID,
		StartPoint: api.Coordinates{Latitude: trip.StartPoint.Latitude, Longitude: trip.StartPoint.Longitude},
		EndPoint:   api.Coordinates{Latitude: trip.EndPoint.Latitude, Longitude: trip.EndPoint.Longitude},
		Price:      trip.Price, Status: api.TripStatus(trip.Status), StartedAt: trip.StartedAt,
		FinishedAt: trip.FinishedAt, LastPositionAt: trip.LastPositionAt,
	}
}

func problemSlug(code string) string {
	result := make([]byte, len(code))
	for index := range code {
		if code[index] == '_' {
			result[index] = '-'
		} else {
			result[index] = code[index]
		}
	}
	return string(result)
}
