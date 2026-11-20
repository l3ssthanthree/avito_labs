package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type exchangeKey struct{}

type exchange struct {
	writer  chimiddleware.WrapResponseWriter
	started time.Time
	route   string
	method  string
	span    trace.SpanContext
	err     error
}

func New(
	router chi.Router,
	logger *slog.Logger,
	traces trace.TracerProvider,
	meter metric.Meter,
	onPanic func(http.ResponseWriter, *http.Request),
) (http.Handler, error) {
	duration, err := meter.Float64Histogram("http.server.request.duration",
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10),
	)
	if err != nil {
		return nil, fmt.Errorf("create HTTP duration histogram: %w", err)
	}
	active, err := meter.Int64UpDownCounter("http.server.active_requests", metric.WithUnit("{request}"))
	if err != nil {
		return nil, fmt.Errorf("create HTTP active requests counter: %w", err)
	}
	measured := measure(duration, active, router)
	logged := logRequests(logger, measured)
	traced := otelhttp.NewHandler(logged, "",
		otelhttp.WithTracerProvider(traces),
		otelhttp.WithMeterProvider(noop.NewMeterProvider()),
		otelhttp.WithPropagators(propagation.TraceContext{}),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			request := requestExchange(r.Context())
			if request.route == "" {
				return request.method
			}
			return request.method + " " + request.route
		}),
	)
	return recoverRequests(router, logger, onPanic, traced), nil
}

func RecordError(ctx context.Context, err error) bool {
	request := requestExchange(ctx)
	if request == nil {
		return false
	}
	request.err = err
	return true
}

func requestExchange(ctx context.Context) *exchange {
	request, _ := ctx.Value(exchangeKey{}).(*exchange)
	return request
}

func recoverRequests(router chi.Router, logger *slog.Logger, onPanic func(http.ResponseWriter, *http.Request), next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := &exchange{
			writer:  chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor),
			started: time.Now(),
			route:   routePattern(router, r),
			method:  requestMethod(r.Method),
		}
		ctx := context.WithValue(r.Context(), exchangeKey{}, request)
		defer func() {
			if recovered := recover(); recovered != nil {
				logCtx := trace.ContextWithSpanContext(ctx, request.span)
				args := request.logArgs(request.status(http.StatusInternalServerError))
				args = append(args, "panic_type", fmt.Sprintf("%T", recovered), "stack", string(debug.Stack()))
				logger.ErrorContext(logCtx, "request panic", args...)
				if request.writer.Status() == 0 {
					onPanic(request.writer, r.WithContext(logCtx))
				}
			}
		}()
		next.ServeHTTP(request.writer, r.WithContext(ctx))
	})
}

func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		request := requestExchange(ctx)
		request.span = trace.SpanContextFromContext(ctx)
		next.ServeHTTP(w, r)
		status := request.status(http.StatusOK)
		level := slog.LevelInfo
		if status >= http.StatusInternalServerError {
			level = slog.LevelError
		}
		logger.Log(ctx, level, "request handled", request.logArgs(status)...)
	})
}

func measure(duration metric.Float64Histogram, active metric.Int64UpDownCounter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		request := requestExchange(ctx)
		span := trace.SpanFromContext(ctx)
		method := attribute.String("http.request.method", request.method)
		route := attribute.String("http.route", request.route)
		span.SetAttributes(route)

		countAttrs := metric.WithAttributes(method, route)
		active.Add(ctx, 1, countAttrs)
		returned := false
		defer func() {
			fallback := http.StatusOK
			if !returned {
				fallback = http.StatusInternalServerError
			}
			status := request.status(fallback)
			statusAttr := attribute.Int("http.response.status_code", status)
			span.SetAttributes(statusAttr)
			if !returned {
				span.SetStatus(codes.Error, "handler panic")
				span.SetAttributes(attribute.String("error.type", "panic"))
			} else if status >= http.StatusBadRequest {
				span.SetStatus(codes.Error, http.StatusText(status))
				span.SetAttributes(attribute.String("error.type", strconv.Itoa(status)))
			}
			duration.Record(ctx, time.Since(request.started).Seconds(), metric.WithAttributes(method, route, statusAttr))
			active.Add(ctx, -1, countAttrs)
		}()
		next.ServeHTTP(w, r)
		returned = true
	})
}

func (e *exchange) status(fallback int) int {
	if status := e.writer.Status(); status != 0 {
		return status
	}
	return fallback
}

func (e *exchange) logArgs(status int) []any {
	args := make([]any, 0, 12)
	args = append(args, "method", e.method, "route", e.route, "status", status, "duration_ms", time.Since(e.started).Milliseconds())
	if e.err != nil {
		args = append(args, "error", e.err)
	}
	return args
}

func routePattern(router chi.Router, r *http.Request) string {
	path := r.URL.RawPath
	if path == "" {
		path = r.URL.Path
	}
	route := chi.NewRouteContext()
	if router.Match(route, r.Method, path) {
		return route.RoutePattern()
	}
	return ""
}

func requestMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodHead, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace:
		return method
	default:
		return "_OTHER"
	}
}
