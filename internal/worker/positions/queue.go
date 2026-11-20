package positions

import (
	"container/heap"
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

type task struct {
	trip   domain.StaleTrip
	scan   trace.SpanContext
	index  int
	cancel context.CancelFunc
}

type priorityHeap []*task

const (
	queued    = "queued"
	duplicate = "duplicate"
	dropped   = "dropped"
)

func (h priorityHeap) Len() int           { return len(h) }
func (h priorityHeap) Less(i, j int) bool { return older(h[i].trip, h[j].trip) }
func (h priorityHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *priorityHeap) Push(value any) {
	entry := value.(*task)
	entry.index = len(*h)
	*h = append(*h, entry)
}

func (h *priorityHeap) Pop() any {
	items := *h
	entry := items[len(items)-1]
	items[len(items)-1] = nil
	*h = items[:len(items)-1]
	entry.index = -1
	return entry
}

func older(a, b domain.StaleTrip) bool {
	if a.LastSeen.Equal(b.LastSeen) {
		return a.ID.String() < b.ID.String()
	}
	return a.LastSeen.Before(b.LastSeen)
}

type queue struct {
	mu       sync.Mutex
	ready    *sync.Cond
	items    priorityHeap
	pending  map[uuid.UUID]*task
	capacity int
	closed   bool
}

func newQueue(capacity int) *queue {
	q := &queue{capacity: capacity, pending: make(map[uuid.UUID]*task)}
	q.ready = sync.NewCond(&q.mu)
	return q
}

func (q *queue) offer(entry *task) (string, *task) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.pending[entry.trip.ID] != nil {
		return duplicate, nil
	}
	if q.closed {
		return dropped, nil
	}
	var evicted *task
	if len(q.items) == q.capacity {
		newest := 0
		for i := 1; i < len(q.items); i++ {
			if older(q.items[newest].trip, q.items[i].trip) {
				newest = i
			}
		}
		if !older(entry.trip, q.items[newest].trip) {
			return dropped, nil
		}
		evicted = heap.Remove(&q.items, newest).(*task)
		delete(q.pending, evicted.trip.ID)
	}
	heap.Push(&q.items, entry)
	q.pending[entry.trip.ID] = entry
	q.ready.Signal()
	return queued, evicted
}

func (q *queue) take(ctx context.Context, timeout time.Duration) (*task, context.Context, context.CancelFunc) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.items) == 0 && !q.closed {
		q.ready.Wait()
	}
	if len(q.items) == 0 || ctx.Err() != nil {
		return nil, nil, nil
	}
	entry := heap.Pop(&q.items).(*task)
	jobCtx, cancel := context.WithTimeout(ctx, timeout)
	entry.cancel = cancel
	return entry, jobCtx, cancel
}

func (q *queue) finish(entry *task) {
	q.mu.Lock()
	defer q.mu.Unlock()
	entry.cancel()
	delete(q.pending, entry.trip.ID)
}

func (q *queue) cancel(id uuid.UUID) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	entry := q.pending[id]
	if entry == nil {
		return false
	}
	if entry.index < 0 {
		entry.cancel()
		return false
	}
	heap.Remove(&q.items, entry.index)
	delete(q.pending, id)
	return true
}

func (q *queue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	q.ready.Broadcast()
}

func (q *queue) size() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return int64(len(q.items))
}

func (q *queue) active() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return int64(len(q.pending) - len(q.items))
}
