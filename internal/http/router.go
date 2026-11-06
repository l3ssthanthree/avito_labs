package http

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	oapimiddleware "github.com/oapi-codegen/nethttp-middleware"

	"github.com/l3ssthanthree/avito_labs/internal/generated/api"
	"github.com/l3ssthanthree/avito_labs/internal/http/health"
	"github.com/l3ssthanthree/avito_labs/internal/http/ready"
	"github.com/l3ssthanthree/avito_labs/internal/http/response"
	"github.com/l3ssthanthree/avito_labs/internal/http/trip/create"
	"github.com/l3ssthanthree/avito_labs/internal/http/trip/finish"
	"github.com/l3ssthanthree/avito_labs/internal/http/trip/get"
)

type Endpoints struct {
	*create.Create
	*get.Get
	*finish.Finish
	*health.Liveness
	*ready.Readiness
}

func NewRouter(endpoints Endpoints, writer *response.Writer) (chi.Router, error) {
	openAPISpec, err := api.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load OpenAPI specification: %w", err)
	}

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.NotFound(writer.RouteNotFound)
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		allow := allowedMethods(router, r)
		if allow == "" {
			writer.RouteNotFound(w, r)
			return
		}
		w.Header().Set("Allow", allow)
		writer.MethodNotAllowed(w, r)
	})
	validate := oapimiddleware.OapiRequestValidatorWithOptions(
		openAPISpec,
		&oapimiddleware.Options{
			DoNotValidateServers: true,
			ErrorHandlerWithOpts: func(
				_ context.Context,
				err error,
				w http.ResponseWriter,
				r *http.Request,
				_ oapimiddleware.ErrorHandlerOpts,
			) {
				writer.RequestError(w, r, err)
			},
		},
	)

	api.HandlerWithOptions(endpoints, api.ChiServerOptions{
		BaseRouter:       router.With(limitRequestBody(writer), validate),
		ErrorHandlerFunc: writer.RequestError,
	})
	return router, nil
}

func allowedMethods(router chi.Routes, r *http.Request) string {
	path := r.URL.RawPath
	if path == "" {
		path = r.URL.Path
	}
	methods := [...]string{
		http.MethodConnect, http.MethodDelete, http.MethodGet, http.MethodHead,
		http.MethodOptions, http.MethodPatch, http.MethodPost, http.MethodPut, http.MethodTrace,
	}
	allowed := make([]string, 0, len(methods))
	for _, method := range methods {
		if router.Match(chi.NewRouteContext(), method, path) {
			allowed = append(allowed, method)
		}
	}
	return strings.Join(allowed, ", ")
}

func limitRequestBody(writer *response.Writer) func(http.Handler) http.Handler {
	const maxRequestBodySize = 1 << 20

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxRequestBodySize {
				writer.PayloadTooLarge(w, r)
				return
			}
			if r.Body != nil && r.Body != http.NoBody {
				r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
				r.GetBody = nil
			}
			next.ServeHTTP(w, r)
		})
	}
}
