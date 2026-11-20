package config

import (
	"strings"
	"testing"
)

func TestPositionsConfig(t *testing.T) {
	values := map[string]string{
		"POSITION_CHECK_INTERVAL": "30s", "POSITION_STALE_AFTER": "60s", "POSITION_WORKERS": "4",
		"POSITION_QUEUE_SIZE": "100", "POSITION_SCAN_BATCH_SIZE": "100", "PUSH_HTTP_URL": "http://localhost:8090", "PUSH_REQUEST_TIMEOUT": "1s",
	}
	for name, value := range values {
		t.Setenv(name, value)
	}
	if _, err := loadPositions(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, value string }{
		{"POSITION_QUEUE_SIZE", "0"},
		{"POSITION_WORKERS", "-1"},
		{"POSITION_SCAN_BATCH_SIZE", "x"},
		{"POSITION_STALE_AFTER", "0s"},
		{"POSITION_CHECK_INTERVAL", ""},
		{"PUSH_REQUEST_TIMEOUT", "-1s"},
		{"PUSH_HTTP_URL", ""},
		{"PUSH_HTTP_URL", "ftp://localhost"},
		{"PUSH_HTTP_URL", "http://user:password@localhost"},
		{"PUSH_HTTP_URL", "http://localhost?token=secret"},
	} {
		t.Run(tc.name+"/"+tc.value, func(t *testing.T) {
			t.Setenv(tc.name, tc.value)
			if _, err := loadPositions(); err == nil || !strings.Contains(err.Error(), tc.name) {
				t.Fatalf("invalid %s accepted: %v", tc.name, err)
			}
		})
	}
}
