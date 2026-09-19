package operationscenter

import (
	"io"
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
