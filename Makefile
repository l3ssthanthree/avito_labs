SHELL := /bin/bash
ENV_FILE ?= .env

.PHONY: generate migrate migrate-down run test build docker-build

generate:
	go tool oapi-codegen -generate types,chi-server \
		-include-operation-ids createTrip,getTrip,finishTrip,health,ready \
		-package api -o api/api.gen.go \
		contracts/openapi/trip-service.openapi.yaml

migrate:
	set -a; source $(ENV_FILE); set +a; \
		go tool goose -dir migrations postgres "$$DATABASE_URL" up

migrate-down:
	set -a; source $(ENV_FILE); set +a; \
		go tool goose -dir migrations postgres "$$DATABASE_URL" down

run:
	set -a; source $(ENV_FILE); set +a; \
		go run ./cmd/trip-service

test:
	go test -race ./...

build:
	go build ./...

docker-build:
	docker build -f deploy/Dockerfile -t trip-service:local .
