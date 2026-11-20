package config

import (
	"errors"
	"fmt"
	"net/url"
)

type TelemetryConfig struct {
	ServiceName string
	Protocol    string
}

func loadTelemetry() (TelemetryConfig, error) {
	service, err := required("OTEL_SERVICE_NAME")
	if err != nil {
		return TelemetryConfig{}, err
	}
	endpoint, err := required("OTEL_EXPORTER_OTLP_ENDPOINT")
	if err != nil {
		return TelemetryConfig{}, err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return TelemetryConfig{}, errors.New("OTEL_EXPORTER_OTLP_ENDPOINT must be an HTTP(S) URL")
	}
	protocol, err := required("OTEL_EXPORTER_OTLP_PROTOCOL")
	if err != nil {
		return TelemetryConfig{}, err
	}
	if protocol != "http/protobuf" && protocol != "grpc" {
		return TelemetryConfig{}, fmt.Errorf("unsupported OTEL_EXPORTER_OTLP_PROTOCOL %q", protocol)
	}
	if _, err := required("OTEL_TRACES_SAMPLER"); err != nil {
		return TelemetryConfig{}, err
	}
	return TelemetryConfig{ServiceName: service, Protocol: protocol}, nil
}
