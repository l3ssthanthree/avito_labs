package positions

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/l3ssthanthree/avito_labs/internal/config"
	domain "github.com/l3ssthanthree/avito_labs/internal/domain/trip"
)

type Pool struct {
	repository   tripRepository
	push         pushGateway
	cfg          config.PositionsConfig
	queryTimeout time.Duration
	clock        Clock
	logger       *slog.Logger
	tracer       trace.Tracer
	queue        *queue
	metrics      metrics
	cancelScan   context.CancelFunc
	cancelJobs   context.CancelFunc
	done         chan struct{}
}

func New(repository tripRepository, push pushGateway, cfg config.PositionsConfig, queryTimeout time.Duration,
	clock Clock, logger *slog.Logger, traces trace.TracerProvider, meter metric.Meter,
) (*Pool, error) {
	q := newQueue(cfg.QueueSize)
	m, err := newMetrics(meter, q)
	if err != nil {
		return nil, fmt.Errorf("create position metrics: %w", err)
	}
	return &Pool{
		repository: repository, push: push, cfg: cfg, queryTimeout: queryTimeout, clock: clock,
		logger: logger, tracer: traces.Tracer("tripgo/positions"), queue: q, metrics: m, done: make(chan struct{}),
	}, nil
}

func (p *Pool) Start(ctx context.Context) {
	scanCtx, cancelScan := context.WithCancel(ctx)
	jobsCtx, cancelJobs := context.WithCancel(context.WithoutCancel(ctx))
	p.cancelScan, p.cancelJobs = cancelScan, cancelJobs
	go func() {
		defer close(p.done)
		defer cancelJobs()
		var workers sync.WaitGroup
		for range p.cfg.Workers {
			workers.Go(func() { p.work(jobsCtx) })
		}
		p.runScanner(scanCtx)
		p.queue.close()
		workers.Wait()
	}()
}

func (p *Pool) Stop(ctx context.Context) error {
	if p.cancelScan == nil {
		return nil
	}
	p.cancelScan()
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		p.cancelJobs()
		return fmt.Errorf("drain position workers: %w", ctx.Err())
	}
}

func (p *Pool) Cancel(ctx context.Context, id uuid.UUID) {
	if p.queue.cancel(id) {
		p.metrics.requests.Add(ctx, 1, result("cancelled"))
	}
}

func (p *Pool) runScanner(ctx context.Context) {
	ticker := p.clock.NewTicker(p.cfg.CheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			if ctx.Err() == nil {
				p.scan(ctx)
			}
		}
	}
}

func (p *Pool) scan(ctx context.Context) {
	ctx, span := p.tracer.Start(ctx, "positions.scan")
	defer span.End()
	now := p.clock.Now()
	var cursor *domain.StaleTrip
	var read int64
	defer func() { p.metrics.rows.Record(ctx, read) }()
	for ctx.Err() == nil {
		queryCtx, cancel := context.WithTimeout(ctx, p.queryTimeout)
		trips, err := p.repository.FindStale(queryCtx, now, p.cfg.StaleAfter, cursor, p.cfg.ScanBatchSize)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			span.SetStatus(codes.Error, "stale trip query failed")
			p.logger.ErrorContext(ctx, "scan stale trips", "error", err)
			return
		}
		read += int64(len(trips))
		for _, trip := range trips {
			if ctx.Err() != nil {
				return
			}
			outcome, evicted := p.queue.offer(&task{trip: trip, scan: span.SpanContext()})
			p.metrics.checks.Add(ctx, 1, result(outcome))
			if evicted != nil {
				p.metrics.checks.Add(ctx, 1, result(dropped))
				p.logger.WarnContext(ctx, "position request evicted from full queue")
				return
			}
			if outcome == dropped {
				p.logger.WarnContext(ctx, "position request queue full")
				return
			}
		}
		if len(trips) < p.cfg.ScanBatchSize {
			return
		}
		last := trips[len(trips)-1]
		cursor = &last
	}
}

func (p *Pool) work(ctx context.Context) {
	for {
		entry, jobCtx, cancel := p.queue.take(ctx, p.cfg.PushTimeout)
		if entry == nil {
			return
		}
		p.process(jobCtx, entry)
		cancel()
		p.queue.finish(entry)
	}
}

func (p *Pool) process(ctx context.Context, entry *task) {
	ctx, span := p.tracer.Start(ctx, "positions.request", trace.WithNewRoot(), trace.WithLinks(trace.Link{SpanContext: entry.scan}))
	defer span.End()
	started := p.clock.Now()
	needed, err := p.repository.NeedsPosition(ctx, entry.trip.ID, started, p.cfg.StaleAfter)
	outcome := "cancelled"
	if err == nil && needed {
		started = p.clock.Now()
		err = p.push.RequestPosition(ctx, entry.trip.ID, entry.trip.DriverID)
		outcome = "success"
	}
	if err != nil {
		outcome = "error"
		span.SetStatus(codes.Error, "position request failed")
		p.logger.WarnContext(ctx, "request position failed", "error", err)
	}
	p.metrics.requests.Add(ctx, 1, result(outcome))
	if needed {
		p.metrics.duration.Record(ctx, p.clock.Now().Sub(started).Seconds(), result(outcome))
	}
}
