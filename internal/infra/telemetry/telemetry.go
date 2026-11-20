package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"

	"github.com/l3ssthanthree/avito_labs/internal/config"
)

type Telemetry struct {
	Logger *slog.Logger
	Traces *sdktrace.TracerProvider
	Meters *sdkmetric.MeterProvider
}

func New(ctx context.Context, cfg config.TelemetryConfig, level string) (*Telemetry, error) {
	var logLevel slog.Level
	if err := logLevel.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("parse LOG_LEVEL: %w", err)
	}
	logger := slog.New(traceHandler{Handler: slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})}).
		With("service", cfg.ServiceName)

	res, err := resource.New(ctx, resource.WithAttributes(
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(buildVersion()),
	), resource.WithFromEnv())
	if err != nil {
		return nil, fmt.Errorf("create OpenTelemetry resource: %w", err)
	}

	traces, err := traceExporter(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}
	metrics, err := metricExporter(ctx, cfg)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("create OTLP metric exporter: %w", err), traces.Shutdown(ctx))
	}
	return &Telemetry{
		Logger: logger,
		Traces: sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithBatcher(traces)),
		Meters: sdkmetric.NewMeterProvider(
			sdkmetric.WithResource(res),
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metrics)),
		),
	}, nil
}

func (t *Telemetry) Close(ctx context.Context) error {
	return errors.Join(t.Meters.Shutdown(ctx), t.Traces.Shutdown(ctx))
}

func traceExporter(ctx context.Context, cfg config.TelemetryConfig) (sdktrace.SpanExporter, error) {
	if cfg.Protocol == "grpc" {
		return otlptracegrpc.New(ctx)
	}
	return otlptracehttp.New(ctx)
}

func metricExporter(ctx context.Context, cfg config.TelemetryConfig) (sdkmetric.Exporter, error) {
	if cfg.Protocol == "grpc" {
		return otlpmetricgrpc.New(ctx)
	}
	return otlpmetrichttp.New(ctx)
}

func buildVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "devel"
}
