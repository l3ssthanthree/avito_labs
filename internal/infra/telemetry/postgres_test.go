package telemetry_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/l3ssthanthree/avito_labs/internal/infra/telemetry"
)

func TestPostgresTelemetry(t *testing.T) {
	t.Parallel()
	const secret = "private-data"
	reader := sdkmetric.NewManualReader()
	meters := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	recorder := tracetest.NewSpanRecorder()
	traces := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer func() {
		if err := meters.Shutdown(t.Context()); err != nil {
			t.Error(err)
		}
		if err := traces.Shutdown(t.Context()); err != nil {
			t.Error(err)
		}
	}()
	tracer, err := telemetry.NewPostgresTracer(traces, meters.Meter("test"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, parent := traces.Tracer("test").Start(t.Context(), "request")
	queryCtx := tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{
		SQL: "INSERT INTO trips (id) VALUES ($1)", Args: []any{secret},
	})
	tracer.TraceQueryEnd(queryCtx, nil, pgx.TraceQueryEndData{Err: &pgconn.PgError{Code: "23505", Message: secret}})
	parent.End()

	ended := recorder.Ended()
	if len(ended) != 2 || ended[0].Name() != "INSERT trips" || !ended[0].Parent().Equal(parent.SpanContext()) ||
		ended[0].Status().Code != codes.Error || strings.Contains(ended[0].Status().Description, secret) {
		t.Fatalf("unexpected DB spans: %+v", ended)
	}
	for _, event := range ended[0].Events() {
		for _, attr := range event.Attributes {
			if strings.Contains(attr.Value.String(), secret) {
				t.Fatalf("private error detail in span event: %s", attr)
			}
		}
	}

	var result metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.ScopeMetrics) != 1 || len(result.ScopeMetrics[0].Metrics) != 1 {
		t.Fatalf("expected one DB metric, got %+v", result.ScopeMetrics)
	}
	m := result.ScopeMetrics[0].Metrics[0]
	data, ok := m.Data.(metricdata.Histogram[float64])
	if m.Name != "db.client.operation.duration" || m.Unit != "s" || !ok || len(data.DataPoints) != 1 {
		t.Fatalf("unexpected DB metric: %+v", m)
	}
	point := data.DataPoints[0]
	attrs := point.Attributes.ToSlice()
	if point.Count != 1 || !slices.Contains(point.Bounds, 0.001) ||
		!slices.Contains(attrs, attribute.String("db.operation.name", "INSERT")) ||
		!slices.Contains(attrs, attribute.String("db.collection.name", "trips")) ||
		!slices.Contains(attrs, attribute.String("error.type", "23505")) {
		t.Fatalf("unexpected DB histogram point: %+v", point)
	}
}
