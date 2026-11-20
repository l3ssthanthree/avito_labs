package config

import (
	"fmt"
	"net/url"
	"time"
)

type PositionsConfig struct {
	CheckInterval time.Duration
	StaleAfter    time.Duration
	Workers       int
	QueueSize     int
	ScanBatchSize int
	PushURL       string
	PushTimeout   time.Duration
}

func loadPositions() (PositionsConfig, error) {
	var cfg PositionsConfig
	for name, target := range map[string]*time.Duration{
		"POSITION_CHECK_INTERVAL": &cfg.CheckInterval,
		"POSITION_STALE_AFTER":    &cfg.StaleAfter,
		"PUSH_REQUEST_TIMEOUT":    &cfg.PushTimeout,
	} {
		value, err := duration(name)
		if err != nil {
			return cfg, err
		}
		*target = value
	}
	for name, target := range map[string]*int{
		"POSITION_WORKERS":         &cfg.Workers,
		"POSITION_QUEUE_SIZE":      &cfg.QueueSize,
		"POSITION_SCAN_BATCH_SIZE": &cfg.ScanBatchSize,
	} {
		value, err := integer(name)
		if err != nil {
			return cfg, err
		}
		if value <= 0 {
			return cfg, fmt.Errorf("%s must be positive", name)
		}
		*target = int(value)
	}
	raw, err := required("PUSH_HTTP_URL")
	if err != nil {
		return cfg, err
	}
	endpoint, err := url.Parse(raw)
	if err != nil || endpoint.Hostname() == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return cfg, fmt.Errorf("PUSH_HTTP_URL must be an absolute HTTP or HTTPS URL without credentials, query or fragment")
	}
	cfg.PushURL = raw
	return cfg, nil
}
