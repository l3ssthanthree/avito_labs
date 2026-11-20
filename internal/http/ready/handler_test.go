package ready_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/l3ssthanthree/avito_labs/internal/http/ready"
	"github.com/l3ssthanthree/avito_labs/internal/http/response"
)

func TestReady(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body string
		pingErr    error
		cancelled  bool
		status     int
	}{
		{"database available", `{"status":"ok"}`, nil, false, http.StatusOK},
		{"database down", `{"status":"unavailable"}`, errors.New("private database address"), false, http.StatusServiceUnavailable},
		{"request cancelled", `{"status":"unavailable"}`, context.Canceled, true, http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			db := NewMockdatabasePinger(gomock.NewController(t))
			db.EXPECT().Ping(gomock.Any()).DoAndReturn(func(pingCtx context.Context) error {
				if _, ok := pingCtx.Deadline(); !ok || !errors.Is(pingCtx.Err(), ctx.Err()) {
					t.Fatalf("ping did not inherit cancellation or set a deadline: %v", pingCtx.Err())
				}
				return tc.pingErr
			})
			writer := response.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/ready", nil)
			rec := httptest.NewRecorder()
			ready.New(db, time.Second, writer).Ready(rec, req)
			if rec.Code != tc.status || strings.TrimSpace(rec.Body.String()) != tc.body {
				t.Fatalf("HTTP %d, body %s, want %d and %s", rec.Code, rec.Body, tc.status, tc.body)
			}
		})
	}
}
