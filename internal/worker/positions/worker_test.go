package positions

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/l3ssthanthree/avito_labs/internal/config"
	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func testPool(t *testing.T, workers, capacity int) (*Pool, *MocktripRepository, *MockpushGateway, *MockClock, *MockTicker) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo, push, clock, ticker := NewMocktripRepository(ctrl), NewMockpushGateway(ctrl), NewMockClock(ctrl), NewMockTicker(ctrl)
	clock.EXPECT().Now().Return(time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)).AnyTimes()
	cfg := config.PositionsConfig{
		Workers: workers, QueueSize: capacity, ScanBatchSize: 100,
		CheckInterval: 30 * time.Second, StaleAfter: time.Minute, PushTimeout: time.Second,
	}
	p, err := New(repo, push, cfg, time.Second, clock, slog.New(slog.NewTextHandler(io.Discard, nil)), tracenoop.NewTracerProvider(), noop.NewMeterProvider().Meter("test"))
	if err != nil {
		t.Fatal(err)
	}
	return p, repo, push, clock, ticker
}

func stale(at time.Time) domain.StaleTrip {
	return domain.StaleTrip{ID: uuid.New(), DriverID: uuid.New(), LastSeen: at}
}

func TestPriorityQueue(t *testing.T) {
	t.Parallel()
	now := time.Now()
	old, middle, fresh := stale(now.Add(-3*time.Hour)), stale(now.Add(-2*time.Hour)), stale(now.Add(-time.Hour))
	q := newQueue(2)
	for _, trip := range []domain.StaleTrip{fresh, middle} {
		if outcome, _ := q.offer(&task{trip: trip}); outcome != queued {
			t.Fatal(outcome)
		}
	}
	if outcome, _ := q.offer(&task{trip: middle}); outcome != duplicate {
		t.Fatal(outcome)
	}
	if outcome, evicted := q.offer(&task{trip: old}); outcome != queued || evicted.trip.ID != fresh.ID {
		t.Fatalf("outcome %s, evicted %+v", outcome, evicted)
	}
	if outcome, _ := q.offer(&task{trip: fresh}); outcome != dropped {
		t.Fatal(outcome)
	}
	for _, id := range []uuid.UUID{old.ID, middle.ID} {
		entry, _, cancel := q.take(t.Context(), time.Second)
		cancel()
		if entry.trip.ID != id {
			t.Fatalf("got %v, want %v", entry.trip.ID, id)
		}
		if outcome, _ := q.offer(&task{trip: entry.trip}); outcome != duplicate {
			t.Fatal("in-flight task lost deduplication")
		}
		q.finish(entry)
	}
	if q.size() != 0 || q.active() != 0 {
		t.Fatal("queue not empty")
	}
	q.close()
	if entry, _, _ := q.take(t.Context(), time.Second); entry != nil {
		t.Fatal("closed queue returned a task")
	}
}

func TestCancellationAndRequeue(t *testing.T) {
	t.Parallel()
	q := newQueue(1)
	trip := stale(time.Now())
	q.offer(&task{trip: trip})
	if !q.cancel(trip.ID) || q.cancel(trip.ID) || q.size() != 0 {
		t.Fatal("pending task not removed")
	}
	q.offer(&task{trip: trip})
	entry, jobCtx, cancel := q.take(t.Context(), time.Second)
	defer cancel()
	q.cancel(trip.ID)
	if jobCtx.Err() == nil {
		t.Fatal("in-flight task context not cancelled")
	}
	q.finish(entry)
	if outcome, _ := q.offer(&task{trip: trip}); outcome != queued {
		t.Fatal(outcome)
	}
}

func TestCancelledQueuedTaskNeverPushes(t *testing.T) {
	t.Parallel()
	p, _, _, _, _ := testPool(t, 1, 1)
	trip := stale(time.Now())
	p.queue.offer(&task{trip: trip})
	p.Cancel(t.Context(), trip.ID)
	p.queue.close()
	p.work(t.Context())
	if p.queue.size() != 0 || p.queue.active() != 0 {
		t.Fatal("cancelled task survived drain")
	}
}

func TestPriorityOrderAcrossTicksAndEqualTimes(t *testing.T) {
	t.Parallel()
	now := time.Now()
	old, middle, fresh := stale(now.Add(-3*time.Hour)), stale(now.Add(-2*time.Hour)), stale(now.Add(-time.Hour))
	tied := middle
	middle.ID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	tied.ID = uuid.MustParse("00000000-0000-0000-0000-000000000002")
	q := newQueue(4)
	for _, trip := range []domain.StaleTrip{fresh, tied, old, middle} {
		q.offer(&task{trip: trip})
	}
	for _, expected := range []uuid.UUID{old.ID, middle.ID, tied.ID, fresh.ID} {
		entry, _, cancel := q.take(t.Context(), time.Second)
		cancel()
		if entry.trip.ID != expected {
			t.Fatalf("got %v, want %v", entry.trip.ID, expected)
		}
		q.finish(entry)
	}
}

func TestScannerBatchesAndOverflow(t *testing.T) {
	for _, capacity := range []int{300, 50} {
		t.Run(strconv.Itoa(capacity), func(t *testing.T) {
			t.Parallel()
			p, repo, _, _, _ := testPool(t, 1, capacity)
			all := make([]domain.StaleTrip, 250)
			for i := range all {
				all[i] = stale(time.Unix(int64(i), 0))
			}
			var first *domain.StaleTrip
			call := repo.EXPECT().FindStale(gomock.Any(), gomock.Any(), time.Minute, first, 100).Return(all[:100], nil)
			if capacity == 300 {
				call = repo.EXPECT().FindStale(gomock.Any(), gomock.Any(), time.Minute, &all[99], 100).Return(all[100:200], nil).After(call)
				repo.EXPECT().FindStale(gomock.Any(), gomock.Any(), time.Minute, &all[199], 100).Return(all[200:], nil).After(call)
			}
			p.scan(t.Context())
			want := min(capacity, len(all))
			if got := p.queue.size(); got != int64(want) {
				t.Fatalf("queue size %d, want %d", got, want)
			}
		})
	}
}

func TestScannerDedupCancelAndFailure(t *testing.T) {
	t.Parallel()
	p, repo, _, _, _ := testPool(t, 1, 2)
	trip := stale(time.Now())
	repo.EXPECT().FindStale(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), 100).Return([]domain.StaleTrip{trip}, nil).Times(2)
	p.scan(t.Context())
	p.scan(t.Context())
	if p.queue.size() != 1 {
		t.Fatal("duplicate task queued")
	}
	p.Cancel(t.Context(), trip.ID)
	if p.queue.size() != 0 {
		t.Fatal("cancelled trip remains queued")
	}
	repo.EXPECT().FindStale(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), 100).Return(nil, errors.New("db unavailable"))
	p.scan(t.Context())
}

func TestWorkersDrainAndContinueAfterFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, repo, push, clock, ticker := testPool(t, 2, 8)
		ticks := make(chan time.Time)
		clock.EXPECT().NewTicker(30 * time.Second).Return(ticker)
		ticker.EXPECT().C().Return(ticks).AnyTimes()
		ticker.EXPECT().Stop()
		var active, count atomic.Int64
		release := make(chan struct{})
		repo.EXPECT().NeedsPosition(gomock.Any(), gomock.Any(), gomock.Any(), time.Minute).Return(true, nil).Times(6)
		push.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID) error {
			active.Add(1)
			<-release
			active.Add(-1)
			if count.Add(1) == 1 {
				return errors.New("push unavailable")
			}
			return nil
		}).Times(6)
		for i := range 6 {
			p.queue.offer(&task{trip: stale(time.Unix(int64(i), 0))})
		}
		ctx, cancel := context.WithCancel(t.Context())
		p.Start(ctx)
		synctest.Wait()
		if active.Load() != 2 {
			t.Fatalf("workers=%d", active.Load())
		}
		cancel()
		close(release)
		if err := p.Stop(t.Context()); err != nil {
			t.Fatal(err)
		}
		if count.Load() != 6 || p.queue.size() != 0 || p.queue.active() != 0 {
			t.Fatal("shutdown did not drain tasks")
		}
	})
}

func TestCancelledSnapshotDoesNotPush(t *testing.T) {
	t.Parallel()
	p, repo, _, _, _ := testPool(t, 1, 1)
	trip := stale(time.Now())
	p.queue.offer(&task{trip: trip})
	repo.EXPECT().NeedsPosition(gomock.Any(), trip.ID, gomock.Any(), time.Minute).Return(false, nil)
	p.queue.close()
	p.work(t.Context())
}

func TestPushTimeoutAndForcedStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, repo, push, clock, ticker := testPool(t, 1, 1)
		ticks := make(chan time.Time)
		clock.EXPECT().NewTicker(gomock.Any()).Return(ticker)
		ticker.EXPECT().C().Return(ticks).AnyTimes()
		ticker.EXPECT().Stop()
		repo.EXPECT().NeedsPosition(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
		push.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _, _ uuid.UUID) error { <-ctx.Done(); return ctx.Err() })
		p.queue.offer(&task{trip: stale(time.Now())})
		p.Start(t.Context())
		synctest.Wait()
		shutdownCtx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
		defer cancel()
		if err := p.Stop(shutdownCtx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("want shutdown timeout, got %v", err)
		}
		<-p.done
	})
}

func TestWorkerSpanLinksToScan(t *testing.T) {
	t.Parallel()
	p, repo, push, _, _ := testPool(t, 1, 1)
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer func() {
		if err := provider.Shutdown(t.Context()); err != nil {
			t.Error(err)
		}
	}()
	p.tracer = provider.Tracer("test")
	_, scan := p.tracer.Start(t.Context(), "scan")
	scan.End()
	entry := &task{trip: stale(time.Now()), scan: scan.SpanContext()}
	repo.EXPECT().NeedsPosition(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	push.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	p.process(t.Context(), entry)
	spans := recorder.Ended()
	if len(spans) != 2 || spans[1].Parent().IsValid() || len(spans[1].Links()) != 1 || !spans[1].Links()[0].SpanContext.Equal(scan.SpanContext()) {
		t.Fatalf("invalid async span relationship: %+v", spans)
	}
}
