# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w" \
    -o /out/trip-service ./cmd/trip-service

FROM scratch
COPY --from=build --chown=65532:65532 /out/trip-service /trip-service
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/trip-service"]
