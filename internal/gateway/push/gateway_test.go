package push

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	api "github.com/l3ssthanthree/avito_labs/internal/generated/push"
)

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error { b.closed = true; return nil }

func TestPushContractAndErrors(t *testing.T) {
	t.Parallel()
	for _, status := range []int{202, 400, 429, 500, 503, 302} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			client := NewMockhttpClient(gomock.NewController(t))
			generated, err := api.NewClient("http://push.test", api.WithHTTPClient(client))
			if err != nil {
				t.Fatal(err)
			}
			gateway := &Gateway{client: generated}
			tripID, driverID := uuid.New(), uuid.New()
			body := &trackedBody{Reader: strings.NewReader(`{}`)}
			client.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost || req.URL.Path != "/api/v1/push" || req.Header.Get("Content-Type") != "application/json" {
					t.Fatalf("invalid request: %+v", req)
				}
				var payload api.PushRequest
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if payload.Kind != api.REQUESTPOSITION || payload.RecipientId != driverID || payload.RequestId == "" || payload.Data == nil || (*payload.Data)["trip_id"] != tripID.String() {
					t.Fatalf("wrong payload: %+v", payload)
				}
				return &http.Response{StatusCode: status, Body: body}, nil
			})
			err = gateway.RequestPosition(t.Context(), tripID, driverID)
			if status == 202 && err != nil {
				t.Fatal(err)
			}
			if status != 202 {
				var failure *HTTPError
				if !errors.As(err, &failure) || failure.StatusCode != status {
					t.Fatalf("HTTP failure not classified: %v", err)
				}
			}
			if !body.closed {
				t.Fatal("response body not closed")
			}
		})
	}
}

func TestPushTransportFailure(t *testing.T) {
	t.Parallel()
	client := NewMockhttpClient(gomock.NewController(t))
	generated, err := api.NewClient("http://push.test", api.WithHTTPClient(client))
	if err != nil {
		t.Fatal(err)
	}
	client.EXPECT().Do(gomock.Any()).Return(nil, context.DeadlineExceeded)
	gateway := &Gateway{client: generated}
	if err := gateway.RequestPosition(t.Context(), uuid.New(), uuid.New()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

type failedReader struct{ err error }

func (r failedReader) Read(_ []byte) (int, error) { return 0, r.err }

func TestPushReadFailureClosesBody(t *testing.T) {
	t.Parallel()
	client := NewMockhttpClient(gomock.NewController(t))
	generated, err := api.NewClient("http://push.test", api.WithHTTPClient(client))
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("response read failed")
	body := &trackedBody{Reader: failedReader{err: failure}}
	client.EXPECT().Do(gomock.Any()).Return(&http.Response{StatusCode: http.StatusAccepted, Body: body}, nil)
	gateway := &Gateway{client: generated}
	if err := gateway.RequestPosition(t.Context(), uuid.New(), uuid.New()); !errors.Is(err, failure) {
		t.Fatalf("lost read error: %v", err)
	}
	if !body.closed {
		t.Fatal("body not closed on read failure")
	}
}
