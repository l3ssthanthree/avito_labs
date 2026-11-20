package push

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	api "github.com/l3ssthanthree/avito_labs/internal/generated/push"
)

type HTTPError struct{ StatusCode int }

func (e *HTTPError) Error() string { return fmt.Sprintf("push service returned HTTP %d", e.StatusCode) }

type Gateway struct {
	client    *api.Client
	transport *http.Transport
}

func New(endpoint string, timeout time.Duration, traces trace.TracerProvider) (*Gateway, error) {
	transport := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: timeout, KeepAlive: timeout}).DialContext,
		TLSHandshakeTimeout: timeout, ResponseHeaderTimeout: timeout,
		IdleConnTimeout: timeout, MaxIdleConns: 4, MaxIdleConnsPerHost: 4,
	}
	var client httpClient = &http.Client{
		Timeout: timeout, Transport: otelhttp.NewTransport(transport,
			otelhttp.WithTracerProvider(traces), otelhttp.WithPropagators(propagation.TraceContext{})),
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	generated, err := api.NewClient(strings.TrimRight(endpoint, "/"), api.WithHTTPClient(client))
	if err != nil {
		transport.CloseIdleConnections()
		return nil, fmt.Errorf("create push client: %w", err)
	}
	return &Gateway{client: generated, transport: transport}, nil
}

func (g *Gateway) RequestPosition(ctx context.Context, tripID, driverID uuid.UUID) error {
	data := map[string]string{"trip_id": tripID.String()}
	response, err := g.client.SendPush(ctx, api.SendPushJSONRequestBody{
		RequestId: uuid.NewString(), RecipientId: driverID, Kind: api.REQUESTPOSITION, Data: &data,
	})
	if err != nil {
		return fmt.Errorf("send position push: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusAccepted {
		return &HTTPError{StatusCode: response.StatusCode}
	}
	if _, err := io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10)); err != nil {
		return fmt.Errorf("read push response: %w", err)
	}
	return nil
}

func (g *Gateway) Close() { g.transport.CloseIdleConnections() }
