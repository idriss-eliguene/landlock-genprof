package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8stesting "k8s.io/client-go/testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
)

func TestV08EnvironmentRoutesAreReadOnlyAndBounded(t *testing.T) {
	client, reads := workbenchReadFixture(t, "default")
	server, err := newWorkbenchServer(reads, "", 18080)
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://localhost:18080/api/v08/environment", nil)
	request.Host = "127.0.0.1:18080"
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"totalCount"`) {
		t.Fatalf("environment response: status=%d body=%s", response.Code, response.Body.String())
	}
	listCalls := 0
	fakeClient, ok := client.(interface{ Actions() []k8stesting.Action })
	if !ok {
		t.Fatal("test dynamic client does not expose actions")
	}
	for _, action := range fakeClient.Actions() {
		if action.GetVerb() == "list" {
			listCalls++
		}
	}
	if listCalls != 5 {
		t.Fatalf("bulk list calls=%d, want 5", listCalls)
	}

	post := httptest.NewRequest(http.MethodPost, "http://localhost:18080/api/v08/environment", nil)
	post.Host = "127.0.0.1:18080"
	postResponse := httptest.NewRecorder()
	server.ServeHTTP(postResponse, post)
	if postResponse.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status=%d, want 405", postResponse.Code)
	}
}

func TestV08SubjectValidationAndUnknownDetail(t *testing.T) {
	_, reads := workbenchReadFixture(t, "default")
	server, err := newWorkbenchServer(reads, "", 18080)
	if err != nil {
		t.Fatal(err)
	}

	bad := httptest.NewRequest(http.MethodGet, "http://localhost:18080/api/v08/environment/detail?scope=CONTAINER&target=Deployment%2Fapi&container=app&imageIdentity=bad&binaryPath=", nil)
	bad.Host = "127.0.0.1:18080"
	badResponse := httptest.NewRecorder()
	server.ServeHTTP(badResponse, bad)
	if badResponse.Code != http.StatusBadRequest {
		t.Fatalf("malformed subject status=%d", badResponse.Code)
	}

	unknown := httptest.NewRequest(http.MethodGet, "http://localhost:18080/api/v08/environment/detail?scope=CONTAINER&target=Deployment%2Fapi&container=app&imageIdentity=sha256%2Faaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&binaryPath=", nil)
	unknown.Host = "127.0.0.1:18080"
	unknownResponse := httptest.NewRecorder()
	server.ServeHTTP(unknownResponse, unknown)
	if unknownResponse.Code != http.StatusBadRequest {
		t.Fatalf("malformed digest status=%d", unknownResponse.Code)
	}
}

func TestV08RequiredListFailureDoesNotBecomeEmptySuccess(t *testing.T) {
	_, reads := workbenchReadFixture(t, "default")
	server, err := newWorkbenchServer(v08ListFailureReads{WorkbenchReadCapability: reads}, "", 18080)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://localhost:18080/api/v08/environment", nil)
	request.Host = "127.0.0.1:18080"
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("failure status=%d body=%s", response.Code, response.Body.String())
	}
}

type v08ListFailureReads struct {
	k8s.WorkbenchReadCapability
}

func (v v08ListFailureReads) ListTrainingHistory(_ context.Context) (*unstructured.UnstructuredList, error) {
	return nil, &k8s.ReadError{State: k8s.ReadPermissionDenied, Resource: "traininghistories", Err: errors.New("denied")}
}
