package observability

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoggerRedactsCredentialMaterial(t *testing.T) {
	var out bytes.Buffer
	logger, err := NewLogger(&out, "INFO")
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("authentication_failed", map[string]interface{}{
		"secret":        "hmac-secret",
		"signature":     "raw-signature",
		"authorization": "Bearer token",
		"token":         "service-account-token",
		"reason":        errors.New(strings.Repeat("x", 600)),
	})
	got := out.String()
	for _, forbidden := range []string{"hmac-secret", "raw-signature", "Bearer token", "service-account-token"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("log leaked %q: %s", forbidden, got)
		}
	}
	if !strings.Contains(got, "[REDACTED]") || len(got) > 1400 {
		t.Fatalf("redaction or bounded error failed: %s", got)
	}
}

func TestRequestIDAndRouteAreBounded(t *testing.T) {
	if RequestID("client-safe-id") != "client-safe-id" {
		t.Fatal("valid request ID was not preserved")
	}
	if got := RequestID(strings.Repeat("x", 65)); len(got) == 65 {
		t.Fatal("oversized request ID was accepted")
	}
	if got := Route("/api/governance/proposals/secret-name/approve"); got != "/api/governance/:id" {
		t.Fatalf("route was not normalized: %q", got)
	}
	if got := Route("/arbitrary/secret"); got != "/other" {
		t.Fatalf("unknown route was not bounded: %q", got)
	}
}

func TestMetricsExposeBoundedLabels(t *testing.T) {
	metrics := NewMetrics()
	metrics.HTTP("GET", Route("/api/governance/proposals/private/approve"), 401)
	metrics.HTTPDuration(0.25, "GET", "/api/governance/proposals/:id")
	metrics.AuthFailure("AUTH_STALE")
	metrics.ExecutorFailed("EXECUTOR_LOST")
	response := httptest.NewRecorder()
	metrics.ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	got := response.Body.String()
	for _, forbidden := range []string{"private", "request_id", "uid", "filesystem", "raw-error"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("metric output leaked unbounded label/value %q: %s", forbidden, got)
		}
	}
	for _, required := range []string{"operations_center_http_requests_total", "operations_center_http_request_duration_seconds_sum", "operations_center_authentication_failures_total", "observation_executor_failed_total"} {
		if !strings.Contains(got, required) {
			t.Fatalf("metric %q missing: %s", required, got)
		}
	}
}

func TestConfigFailsClosedForInvalidObservabilityConfiguration(t *testing.T) {
	t.Setenv("LANDLOCK_GENPROF_LOG_LEVEL", "INVALID")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("invalid log level was accepted")
	}
	t.Setenv("LANDLOCK_GENPROF_LOG_LEVEL", "INFO")
	t.Setenv("LANDLOCK_GENPROF_METRICS_ENABLED", "true")
	t.Setenv("LANDLOCK_GENPROF_METRICS_PORT", "")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("enabled metrics without a port was accepted")
	}
}
