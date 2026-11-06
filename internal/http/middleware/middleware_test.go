package middleware_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/l3ssthanthree/avito_labs/internal/generated/api"
	"github.com/l3ssthanthree/avito_labs/internal/http/middleware"
	"github.com/l3ssthanthree/avito_labs/internal/http/response"
)

func TestRecoverKeepsRouteAndTrace(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		write  bool
		status int
	}{
		{"before headers", false, http.StatusInternalServerError},
		{"after headers", true, http.StatusAccepted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testRecover(t, tc.write, tc.status)
		})
	}
}

func testRecover(t *testing.T, write bool, status int) {
	t.Helper()
	const pattern = "/panic/{id}"
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	router := chi.NewRouter()
	router.Get(pattern, func(w http.ResponseWriter, _ *http.Request) {
		if write {
			w.WriteHeader(http.StatusAccepted)
		}
		panic("private panic value")
	})
	recorder := tracetest.NewSpanRecorder()
	traces := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	reader := sdkmetric.NewManualReader()
	meters := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer func() {
		if err := traces.Shutdown(t.Context()); err != nil {
			t.Error(err)
		}
		if err := meters.Shutdown(t.Context()); err != nil {
			t.Error(err)
		}
	}()
	handler, err := middleware.New(router, logger, traces, meters.Meter("test"), response.New(logger).Panic)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/panic/123", nil))
	if rec.Code != status {
		t.Fatalf("got HTTP %d, %s", rec.Code, rec.Body)
	}
	if write {
		if rec.Body.Len() != 0 {
			t.Fatalf("response already started, unexpected body: %s", rec.Body)
		}
	} else {
		var problem api.Problem
		if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil || problem.Code != "internal_error" {
			t.Fatalf("unexpected problem: %s", rec.Body)
		}
	}
	ended := recorder.Ended()
	if len(ended) != 1 || ended[0].Name() != "GET "+pattern || ended[0].Status().Code != codes.Error ||
		!slices.Contains(ended[0].Attributes(), attribute.Int("http.response.status_code", status)) ||
		!slices.Contains(ended[0].Attributes(), attribute.String("error.type", "panic")) {
		t.Fatalf("unexpected spans: %+v", ended)
	}
	var log struct {
		Level  string `json:"level"`
		Status int    `json:"status"`
		Stack  string `json:"stack"`
	}
	if err := json.Unmarshal(logs.Bytes(), &log); err != nil || log.Level != "ERROR" || log.Status != status ||
		log.Stack == "" || bytes.Contains(logs.Bytes(), []byte("private panic value")) {
		t.Fatalf("unexpected panic log: %s", &logs)
	}
	assertPanicMetrics(t, reader, pattern, status)
}

func assertPanicMetrics(t *testing.T, reader *sdkmetric.ManualReader, pattern string, status int) {
	t.Helper()
	var result metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.ScopeMetrics) != 1 || len(result.ScopeMetrics[0].Metrics) != 2 {
		t.Fatalf("expected two HTTP metrics, got %+v", result.ScopeMetrics)
	}
	for _, m := range result.ScopeMetrics[0].Metrics {
		switch m.Name {
		case "http.server.request.duration":
			data, ok := m.Data.(metricdata.Histogram[float64])
			if !ok || len(data.DataPoints) != 1 {
				t.Fatalf("unexpected HTTP histogram: %+v", m.Data)
			}
			point := data.DataPoints[0]
			attrs := point.Attributes.ToSlice()
			if point.Count != 1 || !slices.Contains(point.Bounds, 0.005) ||
				!slices.Contains(attrs, attribute.Int("http.response.status_code", status)) ||
				!slices.Contains(attrs, attribute.String("http.route", pattern)) {
				t.Fatalf("unexpected histogram point: %+v", point)
			}
		case "http.server.active_requests":
			data, ok := m.Data.(metricdata.Sum[int64])
			if !ok || len(data.DataPoints) != 1 || data.DataPoints[0].Value != 0 {
				t.Fatalf("active requests counter not balanced: %+v", m.Data)
			}
		default:
			t.Fatalf("unexpected metric %s", m.Name)
		}
	}
}
