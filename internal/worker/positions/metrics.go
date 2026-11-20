package positions

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type metrics struct {
	checks   metric.Int64Counter
	requests metric.Int64Counter
	duration metric.Float64Histogram
	rows     metric.Int64Histogram
}

func newMetrics(meter metric.Meter, q *queue) (metrics, error) {
	var m metrics
	var err error
	m.checks, err = meter.Int64Counter("tripgo.stale_checks")
	if err != nil {
		return m, err
	}
	m.requests, err = meter.Int64Counter("tripgo.position_requests")
	if err != nil {
		return m, err
	}
	m.duration, err = meter.Float64Histogram("tripgo.position_requests.duration", metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10))
	if err != nil {
		return m, err
	}
	m.rows, err = meter.Int64Histogram("tripgo.stale_checks.rows_read")
	if err != nil {
		return m, err
	}
	_, err = meter.Int64ObservableGauge("tripgo.position_requests.workers.active",
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error { o.Observe(q.active()); return nil }))
	if err != nil {
		return m, err
	}
	_, err = meter.Int64ObservableGauge("tripgo.position_requests.queue.size",
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error { o.Observe(q.size()); return nil }))
	if err != nil {
		return m, fmt.Errorf("observe position queue size: %w", err)
	}
	return m, nil
}

func result(value string) metric.MeasurementOption {
	return metric.WithAttributes(attribute.String("result", value))
}
