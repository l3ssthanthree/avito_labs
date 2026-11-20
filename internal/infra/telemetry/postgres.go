package telemetry

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

const (
	dbOperation  = attribute.Key("db.operation.name")
	dbCollection = attribute.Key("db.collection.name")
	errorType    = attribute.Key("error.type")
)

type queryKey struct{}

type query struct {
	started    time.Time
	operation  string
	collection string
}

type postgresTracer struct {
	tracer   *otelpgx.Tracer
	duration metric.Float64Histogram
}

func NewPostgresTracer(provider trace.TracerProvider, meter metric.Meter) (pgx.QueryTracer, error) {
	duration, err := meter.Float64Histogram("db.client.operation.duration",
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10),
	)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL duration histogram: %w", err)
	}
	tracer := otelpgx.NewTracer(
		otelpgx.WithTracerProvider(provider),
		otelpgx.WithMeterProvider(noop.NewMeterProvider()),
		otelpgx.WithDisableSQLStatementInAttributes(),
		otelpgx.WithDisableConnectionDetailsInAttributes(),
		otelpgx.WithSpanNameCtxFunc(func(ctx context.Context, _ string) string {
			current, _ := ctx.Value(queryKey{}).(query)
			if current.collection == "" {
				return current.operation
			}
			return current.operation + " " + current.collection
		}),
	)
	return &postgresTracer{tracer: tracer, duration: duration}, nil
}

func (t *postgresTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	operation, collection := queryNames(data.SQL)
	current := query{started: time.Now(), operation: operation, collection: collection}
	ctx = context.WithValue(ctx, queryKey{}, current)
	ctx = t.tracer.TraceQueryStart(ctx, conn, data)
	trace.SpanFromContext(ctx).SetAttributes(current.attributes()...)
	return ctx
}

func (t *postgresTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	current, _ := ctx.Value(queryKey{}).(query)
	attrs := current.attributes()
	kind, safeErr := databaseError(data.Err)
	if kind != "" {
		attr := errorType.String(kind)
		attrs = append(attrs, attr)
		trace.SpanFromContext(ctx).SetAttributes(attr)
	}
	t.duration.Record(ctx, time.Since(current.started).Seconds(), metric.WithAttributes(attrs...))
	data.Err = safeErr
	t.tracer.TraceQueryEnd(ctx, conn, data)
}

func (q query) attributes() []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 3)
	attrs = append(attrs, dbOperation.String(q.operation))
	if q.collection != "" {
		attrs = append(attrs, dbCollection.String(q.collection))
	}
	return attrs
}

func databaseError(err error) (string, error) {
	if err == nil || errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr):
		return pgErr.Code, &pgconn.PgError{Code: pgErr.Code, Message: "database operation failed"}
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded", context.DeadlineExceeded
	case errors.Is(err, context.Canceled):
		return "canceled", context.Canceled
	default:
		return "other", errors.New("database operation failed")
	}
}

func queryNames(sql string) (operation, collection string) {
	tokens := strings.Fields(strings.ToLower(sql))
	if len(tokens) == 0 {
		return "PING", ""
	}
	switch tokens[0] {
	case "select", "delete":
		collection = afterKeyword(tokens, "from")
	case "insert":
		collection = afterKeyword(tokens, "into")
	case "update":
		if len(tokens) > 1 {
			collection = tokens[1]
		}
	case "begin", "commit", "rollback":
	default:
		return "OTHER", ""
	}
	collection = strings.Trim(collection, `";`)
	switch collection {
	case "trips", "trip_status_history", "trip_create_idempotency":
	default:
		collection = ""
	}
	return strings.ToUpper(tokens[0]), collection
}

func afterKeyword(tokens []string, keyword string) string {
	if i := slices.Index(tokens, keyword); i >= 0 && i+1 < len(tokens) {
		return tokens[i+1]
	}
	return ""
}
