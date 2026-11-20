package positions

import (
	"context"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

func TestQueueMetrics(t *testing.T) {
	t.Parallel()
	p, repo, _, _, _ := testPool(t, 1, 1)
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer func() {
		if err := provider.Shutdown(t.Context()); err != nil {
			t.Error(err)
		}
	}()
	var err error
	p.metrics, err = newMetrics(provider.Meter("test"), p.queue)
	if err != nil {
		t.Fatal(err)
	}
	trips := []domain.StaleTrip{stale(time.Unix(0, 0)), stale(time.Unix(1, 0))}
	repo.EXPECT().FindStale(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), 100).Return(trips, nil)
	p.scan(t.Context())
	repo.EXPECT().FindStale(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), 100).Return(trips[:1], nil)
	p.scan(t.Context())
	assertMetric(t, reader, "tripgo.position_requests.queue.size", "", 1)
	assertMetric(t, reader, "tripgo.position_requests.workers.active", "", 0)
	assertMetric(t, reader, "tripgo.stale_checks", "queued", 1)
	assertMetric(t, reader, "tripgo.stale_checks", "duplicate", 1)
	assertMetric(t, reader, "tripgo.stale_checks", "dropped", 1)
	p.Cancel(t.Context(), trips[0].ID)
	assertMetric(t, reader, "tripgo.position_requests", "cancelled", 1)
	assertMetric(t, reader, "tripgo.position_requests.queue.size", "", 0)
}

func assertMetric(t *testing.T, reader *sdkmetric.ManualReader, name, outcome string, want int64) {
	t.Helper()
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.WithoutCancel(t.Context()), &data); err != nil {
		t.Fatal(err)
	}
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			var points []metricdata.DataPoint[int64]
			switch m := metric.Data.(type) {
			case metricdata.Sum[int64]:
				points = m.DataPoints
			case metricdata.Gauge[int64]:
				points = m.DataPoints
			}
			for _, point := range points {
				value, _ := point.Attributes.Value("result")
				if value.AsString() == outcome && point.Value == want {
					return
				}
			}
		}
	}
	t.Fatalf("metric %s result=%q did not have value %d: %+v", name, outcome, want, data)
}
