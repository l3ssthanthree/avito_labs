package telemetry

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestLogCorrelationFields(t *testing.T) {
	t.Parallel()
	for _, withSpan := range []bool{false, true} {
		name := "without span"
		if withSpan {
			name = "with span"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			logger := slog.New(traceHandler{Handler: slog.NewJSONHandler(&output, nil)}).With("service", "trip-service")
			ctx := t.Context()
			var span trace.SpanContext
			if withSpan {
				span = trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}})
				ctx = trace.ContextWithSpanContext(ctx, span)
			}
			logger.InfoContext(ctx, "event")
			var record map[string]any
			if err := json.Unmarshal(output.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"time", "level", "msg", "service", "trace_id", "span_id"} {
				if _, ok := record[key]; !ok {
					t.Fatalf("missing %s: %s", key, &output)
				}
			}
			if withSpan && (record["trace_id"] != span.TraceID().String() || record["span_id"] != span.SpanID().String()) {
				t.Fatalf("wrong correlation: %s", &output)
			}
			if !withSpan && (record["trace_id"] != "" || record["span_id"] != "") {
				t.Fatalf("unexpected correlation without span: %s", &output)
			}
		})
	}
}
