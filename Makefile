SHELL := /bin/bash
.SHELLFLAGS := -eo pipefail -c
ENV_DEFAULTS ?= .env.example
ENV_FILE ?= .env
GOLANGCI_LINT ?= go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4

.PHONY: generate migrate migrate-down run test test-integration lint build docker-build

generate:
	go tool oapi-codegen -generate types,chi-server,spec \
		-include-operation-ids createTrip,getTrip,finishTrip,health,ready \
		-package api -o internal/generated/api/api.gen.go \
		contracts/openapi/trip-service.openapi.yaml
	go generate ./internal/...

migrate:
	set -a; source $(ENV_DEFAULTS); source $(ENV_FILE); set +a; \
		go tool goose -dir migrations postgres "$$DATABASE_URL" up

migrate-down:
	set -a; source $(ENV_DEFAULTS); source $(ENV_FILE); set +a; \
		go tool goose -dir migrations postgres "$$DATABASE_URL" down

run:
	set -a; source $(ENV_DEFAULTS); source $(ENV_FILE); set +a; \
		go run ./cmd/trip-service

test:
	go test -race -count=2 -coverpkg=./internal/domain/...,./internal/usecase/... -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -n 1

test-integration:
	set -a; source $(ENV_DEFAULTS); source $(ENV_FILE); set +a; \
		go test -race -count=2 -tags=integration ./internal/repository/...

lint:
	$(GOLANGCI_LINT) run ./...

build:
	go build ./...

docker-build:
	docker build -f deploy/Dockerfile -t trip-service:local .
