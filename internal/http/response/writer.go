package response

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/l3ssthanthree/avito_labs/internal/generated/api"
	"github.com/l3ssthanthree/avito_labs/internal/http/domainerror"
	"github.com/l3ssthanthree/avito_labs/internal/http/middleware"
)

type Writer struct {
	logger *slog.Logger
}

func New(logger *slog.Logger) *Writer {
	return &Writer{logger: logger}
}

func (rw *Writer) Error(w http.ResponseWriter, r *http.Request, err error) {
	status, problem := domainerror.Problem(err)
	if status == http.StatusInternalServerError && !middleware.RecordError(r.Context(), err) {
		rw.logger.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	}
	rw.problem(w, r, status, problem)
}

func (rw *Writer) Panic(w http.ResponseWriter, r *http.Request) {
	status, problem := domainerror.Problem(nil)
	rw.problem(w, r, status, problem)
}

func (rw *Writer) RequestError(w http.ResponseWriter, r *http.Request, err error) {
	var limitErr *http.MaxBytesError
	if errors.As(err, &limitErr) {
		rw.PayloadTooLarge(w, r)
		return
	}
	status, problem := domainerror.InvalidRequest("Request does not match the API contract")
	rw.problem(w, r, status, problem)
}

func (rw *Writer) RouteNotFound(w http.ResponseWriter, r *http.Request) {
	status, problem := domainerror.NewProblem(http.StatusNotFound, "route_not_found", "Route not found", "No route matches the request path")
	rw.problem(w, r, status, problem)
}

func (rw *Writer) MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	status, problem := domainerror.NewProblem(http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "The route does not support this HTTP method")
	rw.problem(w, r, status, problem)
}

func (rw *Writer) PayloadTooLarge(w http.ResponseWriter, r *http.Request) {
	status, problem := domainerror.NewProblem(http.StatusRequestEntityTooLarge, "payload_too_large", "Payload too large", "Request body exceeds the size limit")
	rw.problem(w, r, status, problem)
}

func (rw *Writer) problem(w http.ResponseWriter, r *http.Request, status int, problem api.Problem) {
	instance := r.URL.Path
	problem.Instance = &instance
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(problem); err != nil {
		rw.logger.ErrorContext(r.Context(), "encode problem response", "error", err)
	}
}

func (rw *Writer) JSON(w http.ResponseWriter, r *http.Request, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		rw.logger.ErrorContext(r.Context(), "encode JSON response", "error", err)
	}
}
