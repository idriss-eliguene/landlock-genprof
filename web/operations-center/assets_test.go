package operationscenter

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesEmbeddedMigrationShell(t *testing.T) {
	request := httptest.NewRequest("GET", "/next/", nil)
	recorder := httptest.NewRecorder()
	Handler().ServeHTTP(recorder, request)
	if recorder.Code != 200 {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	body, err := io.ReadAll(recorder.Result().Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if !strings.Contains(string(body), "Operations Center") {
		t.Fatalf("embedded shell does not contain application title")
	}
}

func TestHandlerServesClientRoutesButNotMissingAssets(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantShell  bool
	}{
		{name: "root", path: "/next/", wantStatus: http.StatusOK, wantShell: true},
		{name: "client route", path: "/next/workloads/payments/apps/Deployment/api/api", wantStatus: http.StatusOK, wantShell: true},
		{name: "unknown client route", path: "/next/another-client-route", wantStatus: http.StatusOK, wantShell: true},
		{name: "missing asset", path: "/next/assets/missing.js", wantStatus: http.StatusNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
			body, err := io.ReadAll(recorder.Result().Body)
			if err != nil {
				t.Fatalf("read response: %v", err)
			}
			containsShell := strings.Contains(string(body), "Operations Center")
			if containsShell != test.wantShell {
				t.Fatalf("shell content = %v, want %v", containsShell, test.wantShell)
			}
		})
	}
}
