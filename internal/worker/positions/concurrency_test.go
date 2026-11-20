package positions

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

func TestScannerTicksDoNotOverlap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, repo, _, clock, ticker := testPool(t, 2, 10)
		ticks := make(chan time.Time, 1)
		clock.EXPECT().NewTicker(gomock.Any()).Return(ticker)
		ticker.EXPECT().C().Return(ticks).AnyTimes()
		ticker.EXPECT().Stop()
		release := make(chan struct{})
		var calls atomic.Int64
		repo.EXPECT().FindStale(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), 100).
			DoAndReturn(func(context.Context, time.Time, time.Duration, *domain.StaleTrip, int) ([]domain.StaleTrip, error) {
				calls.Add(1)
				<-release
				return nil, nil
			}).Times(2)
		p.Start(t.Context())
		synctest.Wait()
		ticks <- time.Now()
		synctest.Wait()
		ticks <- time.Now()
		synctest.Wait()
		if calls.Load() != 1 {
			t.Fatalf("parallel scans: %d", calls.Load())
		}
		close(release)
		synctest.Wait()
		if calls.Load() != 2 {
			t.Fatalf("second tick not processed: %d", calls.Load())
		}
		if err := p.Stop(t.Context()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestQueueConcurrentOfferCancelTake(t *testing.T) {
	t.Parallel()
	q := newQueue(10)
	trips := make([]domain.StaleTrip, 20)
	for i := range trips {
		trips[i] = stale(time.Unix(int64(i), 0))
	}
	var workers, producers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			for {
				entry, _, cancel := q.take(t.Context(), time.Second)
				if entry == nil {
					return
				}
				cancel()
				q.finish(entry)
			}
		})
	}
	for range 8 {
		producers.Go(func() {
			for i := range 100 {
				trip := trips[i%len(trips)]
				q.offer(&task{trip: trip})
				if i%3 == 0 {
					q.cancel(trip.ID)
				}
			}
		})
	}
	producers.Wait()
	q.close()
	workers.Wait()
	if q.size() != 0 || q.active() != 0 {
		t.Fatal("pending tasks survived shutdown")
	}
}

func TestWorkerRequestTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, repo, push, _, _ := testPool(t, 1, 1)
		p.queue.offer(&task{trip: stale(time.Now())})
		p.queue.close()
		repo.EXPECT().NeedsPosition(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
		push.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _, _ uuid.UUID) error { <-ctx.Done(); return ctx.Err() })
		done := make(chan struct{})
		go func() { defer close(done); p.work(t.Context()) }()
		<-done
		if p.queue.active() != 0 {
			t.Fatal("timed-out task not cleaned up")
		}
	})
}
