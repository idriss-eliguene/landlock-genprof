package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8stesting "k8s.io/client-go/testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	obskube "github.com/idriss-eliguene/landlock-genprof/internal/observation/kubernetes"
)

func TestV08EnvironmentRoutesAreReadOnlyAndBounded(t *testing.T) {
	client, reads := workbenchReadFixture(t, "default")
	server, err := newWorkbenchServer(reads, 18080)
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
	server, err := newWorkbenchServer(reads, 18080)
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
	server, err := newWorkbenchServer(v08ListFailureReads{WorkbenchReadCapability: reads}, 18080)
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

func testProjectionObservation(name string, malformed bool) *unstructured.Unstructured {
	object := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "landlockgenprof.io/v1alpha1",
		"kind":       "Observation",
		"metadata":   map[string]interface{}{"namespace": "default", "name": name, "uid": name + "-uid"},
		"spec": map[string]interface{}{
			"target": map[string]interface{}{"slot": map[string]interface{}{
				"workload":  map[string]interface{}{"cluster": map[string]interface{}{"namespaceUID": "cluster-uid"}, "namespace": "default", "groupKind": map[string]interface{}{"group": "apps", "kind": "Deployment"}, "name": "api", "uid": "workload-uid"},
				"container": "app",
			}},
			"sources":          []interface{}{"capabilities"},
			"duration":         "1m0s",
			"requesterSession": "g4-test",
		},
	}}
	if malformed {
		object.Object["status"] = map[string]interface{}{"binding": map[string]interface{}{"resolvedTargets": []interface{}{map[string]interface{}{"slot": map[string]interface{}{"workload": map[string]interface{}{}, "container": "app"}}}}}
	} else {
		slot := map[string]interface{}{"workload": map[string]interface{}{"cluster": map[string]interface{}{"namespaceUID": "cluster-uid"}, "namespace": "default", "groupKind": map[string]interface{}{"group": "apps", "kind": "Deployment"}, "name": "api", "uid": "workload-uid"}, "container": "app"}
		object.Object["status"] = map[string]interface{}{
			"binding":   map[string]interface{}{"backend": map[string]interface{}{"kind": "fixture", "version": "v1"}, "resolvedTargets": []interface{}{map[string]interface{}{"slot": slot, "podUID": "pod-uid", "containerID": "containerd://container", "imageRevision": map[string]interface{}{"slot": slot, "imageDigest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}, "imageRevisions": []interface{}{map[string]interface{}{"slot": slot, "imageDigest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}, "targetChanges": []interface{}{}},
			"execution": map[string]interface{}{"state": "REQUESTED", "completion": ""},
			"result":    map[string]interface{}{"sources": []interface{}{}},
		}
	}
	return object
}

func TestV08MalformedObservationIsolatedFromValidProjection(t *testing.T) {
	dyn, reads := workbenchReadFixture(t, "default")
	for _, object := range []*unstructured.Unstructured{testProjectionObservation("valid-observation", false), testProjectionObservation("malformed-observation", true)} {
		if _, err := dyn.Resource(obskube.GVR).Namespace("default").Create(context.Background(), object, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	server, err := newWorkbenchServer(reads, 18080)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://localhost:18080/api/v08/environment", nil)
	request.Host = "127.0.0.1:18080"
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("environment status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Items               []interface{}          `json:"items"`
		ProjectionStatus    string                 `json:"projectionStatus"`
		ExcludedObjectCount int                    `json:"excludedObjectCount"`
		Diagnostics         []projectionDiagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || body.ProjectionStatus != string(projectionDegraded) || body.ExcludedObjectCount != 1 || len(body.Diagnostics) != 1 {
		t.Fatalf("malformed containment response=%s", response.Body.String())
	}
	if body.Diagnostics[0].Name != "malformed-observation" || body.Diagnostics[0].SecurityDisposition != "NOT_ELIGIBLE" {
		t.Fatalf("diagnostic=%#v", body.Diagnostics[0])
	}
	historyRequest := httptest.NewRequest(http.MethodGet, "http://localhost:18080/api/v08/history?scope=CONTAINER&target=Deployment%2Fapi&container=app&imageIdentity=sha256%3Aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&binaryPath=", nil)
	historyRequest.Host = "127.0.0.1:18080"
	historyResponse := httptest.NewRecorder()
	server.ServeHTTP(historyResponse, historyRequest)
	if historyResponse.Code != http.StatusOK || !strings.Contains(historyResponse.Body.String(), `"projectionStatus":"DEGRADED"`) {
		t.Fatalf("history containment status=%d body=%s", historyResponse.Code, historyResponse.Body.String())
	}
}

func TestDirectMalformedObservationReturnsControlledResponse(t *testing.T) {
	dyn, reads := workbenchReadFixture(t, "default")
	object := testProjectionObservation("malformed-direct", true)
	if _, err := dyn.Resource(obskube.GVR).Namespace("default").Create(context.Background(), object, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	server, err := newWorkbenchServer(reads, 18080)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://localhost:18080/api/observations/malformed-direct", nil)
	request.Host = "127.0.0.1:18080"
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "MALFORMED_OBJECT") || strings.Contains(response.Body.String(), "cluster-uid") {
		t.Fatalf("controlled malformed response status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestProjectionDiagnosticsArePerObjectAndDeterministic(t *testing.T) {
	first := testProjectionObservation("z-malformed", true)
	second := testProjectionObservation("a-malformed", true)
	diagnostics := projectionDiagnostics{}
	diagnostics.addMalformed("Observation", "excluded", metadataOf(first), errors.New("invalid container slot"))
	diagnostics.addMalformed("Observation", "excluded", metadataOf(second), errors.New("invalid container slot"))
	diagnostics.finalize()
	if diagnostics.Status != projectionDegraded || diagnostics.ExcludedObjectCount != 2 || len(diagnostics.Diagnostics) != 2 || diagnostics.Diagnostics[0].Name != "a-malformed" {
		t.Fatalf("diagnostics=%#v", diagnostics)
	}
	if _, err := decodeV08Observation(testProjectionObservation("not-valid", true)); err == nil {
		t.Fatal("malformed Observation decoded into a domain object")
	}
}

type v08ListFailureReads struct {
	k8s.WorkbenchReadCapability
}

func (v v08ListFailureReads) ListTrainingHistory(_ context.Context) (*unstructured.UnstructuredList, error) {
	return nil, &k8s.ReadError{State: k8s.ReadPermissionDenied, Resource: "traininghistories", Err: errors.New("denied")}
}
