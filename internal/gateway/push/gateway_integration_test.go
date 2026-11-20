//go:build integration

package push

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestPushClientTimeoutAndPropagation(t *testing.T) {
	t.Parallel()
	received := make(chan http.Header, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		received <- r.Header.Clone()
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	gateway, err := New(server.URL, 200*time.Millisecond, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	defer gateway.Close()
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{1}, TraceFlags: trace.FlagsSampled})
	ctx := trace.ContextWithSpanContext(t.Context(), spanCtx)
	err = gateway.RequestPosition(ctx, uuid.New(), uuid.New())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout was not enforced: %v", err)
	}
	select {
	case headers := <-received:
		if headers.Get("Traceparent") == "" {
			t.Fatal("trace context not propagated")
		}
	default:
		t.Fatal("request did not reach test server before its timeout")
	}
}
