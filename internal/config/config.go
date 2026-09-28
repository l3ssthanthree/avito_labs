package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTP              HTTPConfig
	LogLevel          string
	ShutdownTimeout   time.Duration
	IdempotencyKeyTTL time.Duration
	Database          DatabaseConfig
}

type HTTPConfig struct {
	Addr              string
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

type DatabaseConfig struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	ConnectTimeout  time.Duration
	QueryTimeout    time.Duration
}

func Load() (Config, error) {
	httpAddr, err := required("HTTP_ADDR")
	if err != nil {
		return Config{}, err
	}

	httpReadTimeout, err := duration("HTTP_READ_TIMEOUT")
	if err != nil {
		return Config{}, err
	}

	httpReadHeaderTimeout, err := duration("HTTP_READ_HEADER_TIMEOUT")
	if err != nil {
		return Config{}, err
	}

	httpWriteTimeout, err := duration("HTTP_WRITE_TIMEOUT")
	if err != nil {
		return Config{}, err
	}

	httpIdleTimeout, err := duration("HTTP_IDLE_TIMEOUT")
	if err != nil {
		return Config{}, err
	}

	logLevel, err := required("LOG_LEVEL")
	if err != nil {
		return Config{}, err
	}

	shutdownTimeout, err := duration("SHUTDOWN_TIMEOUT")
	if err != nil {
		return Config{}, err
	}

	idempotencyKeyTTL, err := duration("IDEMPOTENCY_KEY_TTL")
	if err != nil {
		return Config{}, err
	}

	databaseURL, err := required("DATABASE_URL")
	if err != nil {
		return Config{}, err
	}

	maxConns, err := integer("DATABASE_MAX_CONNS")
	if err != nil {
		return Config{}, err
	}

	minConns, err := integer("DATABASE_MIN_CONNS")
	if err != nil {
		return Config{}, err
	}

	maxConnLifetime, err := duration("DATABASE_MAX_CONN_LIFETIME")
	if err != nil {
		return Config{}, err
	}

	connectTimeout, err := duration("DATABASE_CONNECT_TIMEOUT")
	if err != nil {
		return Config{}, err
	}

	queryTimeout, err := duration("DATABASE_QUERY_TIMEOUT")
	if err != nil {
		return Config{}, err
	}

	if maxConns <= 0 {
		return Config{}, fmt.Errorf("DATABASE_MAX_CONNS must be positive")
	}
	if minConns < 0 {
		return Config{}, fmt.Errorf("DATABASE_MIN_CONNS must not be negative")
	}
	if minConns > maxConns {
		return Config{}, fmt.Errorf("DATABASE_MIN_CONNS must not exceed DATABASE_MAX_CONNS")
	}

	return Config{
		HTTP: HTTPConfig{
			Addr:              httpAddr,
			ReadTimeout:       httpReadTimeout,
			ReadHeaderTimeout: httpReadHeaderTimeout,
			WriteTimeout:      httpWriteTimeout,
			IdleTimeout:       httpIdleTimeout,
		},
		LogLevel:          logLevel,
		ShutdownTimeout:   shutdownTimeout,
		IdempotencyKeyTTL: idempotencyKeyTTL,
		Database: DatabaseConfig{
			URL:             databaseURL,
			MaxConns:        maxConns,
			MinConns:        minConns,
			MaxConnLifetime: maxConnLifetime,
			ConnectTimeout:  connectTimeout,
			QueryTimeout:    queryTimeout,
		},
	}, nil
}

func required(name string) (string, error) {
	value, ok := os.LookupEnv(name)
	if !ok || value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func integer(name string) (int32, error) {
	value, err := required(name)
	if err != nil {
		return 0, err
	}

	number, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return int32(number), nil
}

func duration(name string) (time.Duration, error) {
	value, err := required(name)
	if err != nil {
		return 0, err
	}

	result, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	if result <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	return result, nil
}
