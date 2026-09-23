package operationscenter

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesEmbeddedMigrationShell(t *testing.T) {
	request := httptest.NewRequest("GET", "/", nil)
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
		{name: "root", path: "/", wantStatus: http.StatusOK, wantShell: true},
		{name: "client route", path: "/workloads/payments/apps/Deployment/api/api", wantStatus: http.StatusOK, wantShell: true},
		{name: "unknown client route", path: "/another-client-route", wantStatus: http.StatusOK, wantShell: true},
		{name: "missing asset", path: "/assets/missing.js", wantStatus: http.StatusNotFound},
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

func TestHandlerServesApprovedLandlockFavicon(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/landlock-favicon.png", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "image/png") {
		t.Fatalf("content type = %q, want image/png", got)
	}
	if !strings.HasPrefix(string(recorder.Body.Bytes()), "\x89PNG") {
		t.Fatalf("favicon response is not a PNG")
	}
}

func TestRootHandlerRejectsUnsafeMethods(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		recorder := httptest.NewRecorder()
		Handler().ServeHTTP(recorder, httptest.NewRequest(method, "/", nil))
		if recorder.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s / status=%d, want 405", method, recorder.Code)
		}
	}
}
