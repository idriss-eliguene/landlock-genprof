// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	discoveryfake "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/idriss-eliguene/landlock-genprof/internal/association"
	"github.com/idriss-eliguene/landlock-genprof/internal/history"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/projection"
	"github.com/idriss-eliguene/landlock-genprof/internal/workload"
)

// --- Structural / type-level "no write authority" proof (§24) ---
//
// These are not string greps: they inspect the actual Go type structure the
// compiler enforces. If either interface or struct is ever widened to carry
// a write verb, these fail without needing a handler to exist that calls it.

func TestWorkbenchReadCapability_ExposesOnlyBoundedReadMethods(t *testing.T) {
	iface := reflect.TypeOf((*k8s.WorkbenchReadCapability)(nil)).Elem()
	allowed := map[string]bool{
		"SessionIdentity": true, "GetPod": true, "ListPods": true,
		"GetDeployment": true, "GetStatefulSet": true, "GetDaemonSet": true, "GetReplicaSet": true,
		"GetProposal": true, "ListProposals": true, "GetTrainingHistory": true,
		"GetPodLock": true, "GetSPOProfile": true, "ListNetworkPolicies": true,
		"GetApplyAttempt": true, "ListApplyAttempts": true,
		"GetRollbackAttempt": true, "ListRollbackAttempts": true, "GetCustodyEpoch": true,
	}
	if iface.NumMethod() != len(allowed) {
		t.Fatalf("WorkbenchReadCapability has %d methods, want exactly %d — its surface changed", iface.NumMethod(), len(allowed))
	}
	forbidden := []string{"Create", "Update", "Patch", "Delete", "Save", "Approve", "Publish", "SetApproval", "SetStatus"}
	for i := 0; i < iface.NumMethod(); i++ {
		name := iface.Method(i).Name
		if !allowed[name] {
			t.Errorf("WorkbenchReadCapability exposes unexpected method %q: possible write-authority leak into the HTTP boundary", name)
		}
		for _, bad := range forbidden {
			if strings.Contains(name, bad) {
				t.Errorf("WorkbenchReadCapability method %q looks like a write verb", name)
			}
		}
	}
}

func TestWorkbenchUIAcceptsOptionalProposal(t *testing.T) {
	cmd := newWorkbenchCmd()
	if err := cmd.Args(cmd, nil); err != nil {
		t.Fatalf("ui without proposal rejected: %v", err)
	}
	if err := cmd.Args(cmd, []string{"proposal"}); err != nil {
		t.Fatalf("ui with proposal rejected: %v", err)
	}
	if err := cmd.Args(cmd, []string{"one", "two"}); err == nil {
		t.Fatal("ui accepted more than one proposal")
	}
}

func TestWorkbenchServer_HoldsNoWriteCapableKubernetesField(t *testing.T) {
	typ := reflect.TypeOf(workbenchServer{})
	allowedFieldTypes := map[string]bool{
		"k8s.WorkbenchReadCapability": true,
		"*workload.Service":           true,
		"*projection.Service":         true,
		"string":                      true,
		"chan struct {}":              true,
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !allowedFieldTypes[field.Type.String()] {
			t.Errorf("workbenchServer.%s has type %s, not in the allowed read-only set — verify it cannot reach a write operation", field.Name, field.Type)
		}
	}
}

// --- Host validation (§15) ---

func TestWorkbenchValidHost(t *testing.T) {
	const allowed = "127.0.0.1:8080"
	for _, tc := range []struct {
		name string
		host string
		want bool
	}{
		{"exact match", "127.0.0.1:8080", true},
		{"empty", "", false},
		{"wrong port", "127.0.0.1:9999", false},
		{"localhost", "localhost:8080", false},
		{"bare loopback, no port", "127.0.0.1", false},
		{"attacker domain rebound to loopback", "evil.example:8080", false},
		{"wildcard bind address", "0.0.0.0:8080", false},
		{"ipv6 all-interfaces", "[::]:8080", false},
		{"ipv6 loopback", "[::1]:8080", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := workbenchValidHost(tc.host, allowed); got != tc.want {
				t.Errorf("workbenchValidHost(%q, %q) = %v, want %v", tc.host, allowed, got, tc.want)
			}
		})
	}
}

// --- Browser-origin policy (§16) ---

func TestWorkbenchValidBrowserOrigin(t *testing.T) {
	const allowedOrigin = "http://127.0.0.1:8080"
	newReq := func(secFetchSite, origin string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/api/workloads", nil)
		if secFetchSite != "" {
			r.Header.Set("Sec-Fetch-Site", secFetchSite)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		return r
	}
	for _, tc := range []struct {
		name         string
		secFetchSite string
		origin       string
		want         bool
	}{
		{"neither header: non-browser local client", "", "", true},
		{"Sec-Fetch-Site same-origin", "same-origin", "", true},
		{"Sec-Fetch-Site none: direct navigation", "none", "", true},
		{"Sec-Fetch-Site cross-site rejected", "cross-site", "", false},
		{"Sec-Fetch-Site same-site rejected", "same-site", "", false},
		{"Origin exact match", "", allowedOrigin, true},
		{"Origin foreign", "", "http://evil.example", false},
		{"Origin scheme mismatch", "", "https://127.0.0.1:8080", false},
		{"Sec-Fetch-Site wins over a matching Origin", "cross-site", allowedOrigin, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := workbenchValidBrowserOrigin(newReq(tc.secFetchSite, tc.origin), allowedOrigin); got != tc.want {
				t.Errorf("workbenchValidBrowserOrigin() = %v, want %v", got, tc.want)
			}
		})
	}
}

// --- Body rejection (§13) ---

func TestWorkbenchRejectBody(t *testing.T) {
	t.Run("no body accepted", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/workloads", nil)
		w := httptest.NewRecorder()
		if !workbenchRejectBody(w, r) {
			t.Fatalf("empty body was rejected, status=%d", w.Code)
		}
	})
	t.Run("non-empty body rejected", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/workloads", strings.NewReader("mutate"))
		w := httptest.NewRecorder()
		if workbenchRejectBody(w, r) {
			t.Fatal("non-empty body was accepted")
		}
		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})
}

// --- Concurrency bound (§19) ---

func TestWorkbenchAcquireRead_EnforcesConcurrencyBoundFailFast(t *testing.T) {
	s := &workbenchServer{sema: make(chan struct{}, 1)}

	w1 := httptest.NewRecorder()
	release, ok := s.workbenchAcquireRead(w1)
	if !ok {
		t.Fatal("first acquire under the bound was refused")
	}

	w2 := httptest.NewRecorder()
	_, ok = s.workbenchAcquireRead(w2)
	if ok {
		t.Fatal("acquire beyond the bound was granted")
	}
	if w2.Code != http.StatusServiceUnavailable {
		t.Errorf("saturated status = %d, want %d", w2.Code, http.StatusServiceUnavailable)
	}

	release()
	w3 := httptest.NewRecorder()
	if _, ok := s.workbenchAcquireRead(w3); !ok {
		t.Fatal("acquire after release was refused")
	}
}

// --- Request validation / target selector (§13, §6) ---

func TestParseTargetSelector(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query map[string][]string
		want  string // "" means valid
	}{
		{"valid", map[string][]string{"kind": {"Pod"}, "name": {"my-pod"}, "container": {"app"}}, ""},
		{"valid with group", map[string][]string{"group": {"apps"}, "kind": {"Deployment"}, "name": {"api"}, "container": {"app"}}, ""},
		{"missing name", map[string][]string{"kind": {"Pod"}, "container": {"app"}}, "kind, name, and container are required"},
		{"missing container", map[string][]string{"kind": {"Pod"}, "name": {"p"}}, "kind, name, and container are required"},
		{"missing kind", map[string][]string{"name": {"p"}, "container": {"app"}}, "kind, name, and container are required"},
		{"duplicate value", map[string][]string{"kind": {"Pod", "Deployment"}, "name": {"p"}, "container": {"app"}}, "query parameter \"kind\" must appear exactly once"},
		{"unexpected parameter", map[string][]string{"kind": {"Pod"}, "name": {"p"}, "container": {"app"}, "namespace": {"other"}}, "unsupported query parameter \"namespace\""},
		{"invalid name charset", map[string][]string{"kind": {"Pod"}, "name": {"../etc/passwd"}, "container": {"app"}}, "invalid name"},
		{"invalid name uppercase", map[string][]string{"kind": {"Pod"}, "name": {"MyPod"}, "container": {"app"}}, "invalid name"},
		{"invalid kind charset", map[string][]string{"kind": {"Pod;DROP"}, "name": {"p"}, "container": {"app"}}, "invalid kind"},
		{"invalid container charset", map[string][]string{"kind": {"Pod"}, "name": {"p"}, "container": {"app_1"}}, "invalid container"},
		{"oversized name", map[string][]string{"kind": {"Pod"}, "name": {strings.Repeat("a", 300)}, "container": {"app"}}, "identifier too long"},
		{"too many params", map[string][]string{"a": {"1"}, "b": {"2"}, "c": {"3"}, "d": {"4"}, "e": {"5"}, "f": {"6"}, "g": {"7"}, "h": {"8"}, "i": {"9"}}, "too many query parameters"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, verr := parseTargetSelector(tc.query)
			if tc.want == "" {
				if verr != "" {
					t.Fatalf("verr = %q, want valid", verr)
				}
				return
			}
			if verr != tc.want {
				t.Fatalf("verr = %q, want %q", verr, tc.want)
			}
		})
	}
}

// --- Handler-level negative and positive tests, real fixtures (§25, §26) ---

// newTestWorkbenchServer builds a workbenchServer over a bounded
// k8s.WorkbenchReadCapability backed by fake clients, seeded with the given
// Pods before the ReadSession is constructed. It exercises the real
// workload.Service and projection.Service, not a stand-in.
func newTestWorkbenchServer(t *testing.T, namespace string, pods ...*corev1.Pod) (*workbenchServer, string) {
	t.Helper()
	core := kubefake.NewSimpleClientset()
	for _, pod := range pods {
		if _, err := core.CoreV1().Pods(pod.Namespace).Create(context.Background(), pod, metav1.CreateOptions{}); err != nil {
			t.Fatalf("seeding pod %s: %v", pod.Name, err)
		}
	}
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "securityprofileproposals"}: "SecurityProfileProposalList",
		{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}:                 "NetworkPolicyList",
	})
	disc := core.Discovery().(*discoveryfake.FakeDiscovery)
	disc.Resources = []*metav1.APIResourceList{
		{GroupVersion: "landlockgenprof.io/v1alpha1", APIResources: []metav1.APIResource{{Name: "securityprofileproposals"}}},
		{GroupVersion: "networking.k8s.io/v1", APIResources: []metav1.APIResource{{Name: "networkpolicies"}}},
	}
	reads, err := k8s.NewReadSessionForClients(core, dyn, core.Discovery(), namespace)
	if err != nil {
		t.Fatalf("k8s.NewReadSessionForClients() error = %v", err)
	}
	srv, err := newWorkbenchServer(reads, "", 18080)
	if err != nil {
		t.Fatalf("newWorkbenchServer() error = %v", err)
	}
	return srv, "127.0.0.1:18080"
}

func TestWorkbenchServer_UnknownRouteIsNotFound(t *testing.T) {
	srv, host := newTestWorkbenchServer(t, "default")
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	req.Host = host
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestWorkbenchServer_UnsupportedMethodsRejected(t *testing.T) {
	srv, host := newTestWorkbenchServer(t, "default")
	for _, route := range []string{"/", "/api/workloads", "/api/projection"} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			t.Run(method+" "+route, func(t *testing.T) {
				req := httptest.NewRequest(method, route, nil)
				req.Host = host
				w := httptest.NewRecorder()
				srv.ServeHTTP(w, req)
				if w.Code != http.StatusMethodNotAllowed {
					t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
				}
				if got := w.Header().Get("Allow"); got != http.MethodGet {
					t.Errorf("Allow = %q, want %q", got, http.MethodGet)
				}
			})
		}
	}
}

func TestWorkbenchServer_MutationBodyOnGETRejectedBeforeAnyRoute(t *testing.T) {
	srv, host := newTestWorkbenchServer(t, "default")
	req := httptest.NewRequest(http.MethodGet, "/api/workloads", strings.NewReader(`{"approve":true}`))
	req.Host = host
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestWorkbenchServer_BadHostRejected(t *testing.T) {
	srv, _ := newTestWorkbenchServer(t, "default")
	req := httptest.NewRequest(http.MethodGet, "/api/workloads", nil)
	req.Host = "evil.example:18080"
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestWorkbenchServer_ForeignOriginRejected(t *testing.T) {
	srv, host := newTestWorkbenchServer(t, "default")
	req := httptest.NewRequest(http.MethodGet, "/api/workloads", nil)
	req.Host = host
	req.Header.Set("Origin", "http://evil.example")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestWorkbenchServer_CrossSiteFetchMetadataRejected(t *testing.T) {
	srv, host := newTestWorkbenchServer(t, "default")
	req := httptest.NewRequest(http.MethodGet, "/api/workloads", nil)
	req.Host = host
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestWorkbenchServer_NoPermissiveCORSAndSecurityHeadersPresent(t *testing.T) {
	srv, host := newTestWorkbenchServer(t, "default")
	req := httptest.NewRequest(http.MethodGet, "/api/workloads", nil)
	req.Host = host
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want absent", got)
	}
	if got := w.Header().Get("Content-Security-Policy"); got == "" {
		t.Error("Content-Security-Policy header missing")
	} else if strings.Contains(got, "script-src") && !strings.Contains(got, "script-src 'none'") {
		t.Errorf("CSP allows scripts: %q", got)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestWorkbenchServer_ProjectionMalformedTargetRejectedBeforeResolution(t *testing.T) {
	srv, host := newTestWorkbenchServer(t, "default")
	req := httptest.NewRequest(http.MethodGet, "/api/projection?kind=Pod&name=../x&container=app", nil)
	req.Host = host
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestWorkbenchServer_UnknownTargetIsNotFoundNotEmpty(t *testing.T) {
	srv, host := newTestWorkbenchServer(t, "default")
	req := httptest.NewRequest(http.MethodGet, "/api/projection?kind=Pod&name=does-not-exist&container=app", nil)
	req.Host = host
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (not 200-empty)", w.Code, http.StatusNotFound)
	}
	var body workbenchErrorBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding error body: %v", err)
	}
	if body.State == "" {
		t.Error("error body carries no typed state")
	}
}

func TestWorkbenchServer_ValidTargetReturnsProjection(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "wb-pod", Namespace: "default", UID: "uid-1"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
	}
	srv, host := newTestWorkbenchServer(t, "default", pod)
	req := httptest.NewRequest(http.MethodGet, "/api/projection?kind=Pod&name=wb-pod&container=app", nil)
	req.Host = host
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	var body dtoProjection
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding projection body: %v", err)
	}
	if body.Target.Workload.Kind != "Pod" || body.Target.Workload.Name != "wb-pod" || body.Target.Container != "app" {
		t.Errorf("target = %+v, want the resolved GovernedTarget", body.Target)
	}
	// Runtime is EMPTY by fixed decision Q1: no evidence loader in G3.
	if body.Runtime.State != string(projection.Empty) {
		t.Errorf("Runtime.State = %q, want %q (Q1: Evidence is nil)", body.Runtime.State, projection.Empty)
	}
}

func TestWorkbenchServer_WorkloadsRejectsQueryParameters(t *testing.T) {
	srv, host := newTestWorkbenchServer(t, "default")
	req := httptest.NewRequest(http.MethodGet, "/api/workloads?namespace=other", nil)
	req.Host = host
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestWorkbenchServer_WorkloadsListsDiscoveredPod(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "wb-pod", Namespace: "default", UID: "uid-1"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
	}
	srv, host := newTestWorkbenchServer(t, "default", pod)
	req := httptest.NewRequest(http.MethodGet, "/api/workloads", nil)
	req.Host = host
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	var body dtoDiscoveryResult
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding discovery body: %v", err)
	}
	if len(body.Workloads) != 1 || body.Workloads[0].Target.Name != "wb-pod" {
		t.Fatalf("workloads = %+v, want one entry named wb-pod", body.Workloads)
	}
	if len(body.Workloads[0].Pods) != 1 || body.Workloads[0].Pods[0].Containers[0].Target == nil {
		t.Fatalf("discovered container lost its canonical Target: %+v", body.Workloads[0].Pods)
	}
}

func TestWorkbenchClusterPagePreservesNavigationAndSecuritySemantics(t *testing.T) {
	target := k8s.GovernedTarget{Namespace: "default", Workload: k8s.WorkloadRef{Kind: "Pod", Name: "review-pod"}, Container: "app"}
	view := workbenchClusterView{
		Namespace: "default",
		Proposal:  workbenchView{Proposal: "proposal", Lifecycle: "PROPOSAL — structured candidate", CandidateDigest: "sha256:abc", Approval: "DRAFT", ApprovalBinding: "NOT BOUND", Application: "NOT_AVAILABLE — application outcome is not persisted", Verification: "NOT_AVAILABLE — behavioral verification is not persisted"},
		Workloads: []workbenchNavigationWorkload{{Target: target.Workload, Containers: []workbenchNavigationContainer{{Name: "app", Category: "REGULAR", Supported: true, RuntimeState: "STATUS_UNAVAILABLE", Target: &target, Link: "?kind=Pod&name=review-pod&container=app"}}}},
		Selected: &workbenchSelectedTarget{Target: target, RuntimeSubjects: []k8s.RuntimeSubject{{Target: target, PodUID: "uid-1", ImageID: "sha256:image", BinaryPath: "/usr/bin/app"}}, Projection: dtoProjection{
			Declared:     dtoDeclaredConfiguration{dtoSection: dtoSection{State: "AVAILABLE"}},
			Materialized: dtoMaterializedPolicy{dtoSection: dtoSection{State: "UNKNOWN"}, PodLockState: "BACKEND_NOT_INSTALLED"},
			Binding:      dtoBindingEvidence{dtoSection: dtoSection{State: "NOT_AVAILABLE"}},
			Enforcement:  dtoSection{State: "NOT_AVAILABLE"}, BehavioralVerification: dtoSection{State: "NOT_AVAILABLE"},
			Runtime: dtoRuntimeEvidence{dtoSection: dtoSection{State: "EMPTY"}, Excluded: []dtoExcludedEvidence{{Association: dtoAssociationResult{State: "INSUFFICIENT_PROVENANCE", Reason: "legacy evidence"}}}},
			Derived: dtoDerivedPolicy{dtoSection: dtoSection{State: "AVAILABLE"}}, Governance: dtoProposalGovernance{dtoSection: dtoSection{State: "EMPTY"}},
		}},
		NextSteps: []string{"kubectl landlock-genprof approve proposal -n default --expected-digest sha256:abc", "kubectl landlock-genprof apply-proposal proposal -n default"},
	}
	var body bytes.Buffer
	if err := workbenchClusterPageTemplate.Execute(&body, view); err != nil {
		t.Fatalf("cluster template execution = %v", err)
	}
	text := body.String()
	for _, want := range []string{"default", "Pod/review-pod", "app", "AVAILABLE", "UNKNOWN", "EMPTY", "BACKEND_NOT_INSTALLED", "INSUFFICIENT_PROVENANCE", "NOT_AVAILABLE", "kubectl landlock-genprof approve", "kubectl landlock-genprof apply-proposal", "Runtime subject / provenance", "uid-1", "sha256:image", "/usr/bin/app"} {
		if !strings.Contains(text, want) {
			t.Errorf("cluster page omitted semantic content %q", want)
		}
	}
	for _, forbidden := range []string{"<form", "<button", "<script", "Secure", "Protected", "Fully enforced"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("cluster page contains forbidden UI construct/claim %q", forbidden)
		}
	}
}

// TestWorkbenchClusterPageRuntimeSubjectAbsenceIsHonest proves the selected
// view's runtime-subject/provenance panel — one of the eleven concepts
// #186 requires be presented separately — renders an explicit NOT_AVAILABLE
// rather than a silently empty section when discovery found no current
// runtime incarnation for the selected target.
func TestWorkbenchClusterPageRuntimeSubjectAbsenceIsHonest(t *testing.T) {
	target := k8s.GovernedTarget{Namespace: "default", Workload: k8s.WorkloadRef{Kind: "Pod", Name: "review-pod"}, Container: "app"}
	view := workbenchClusterView{
		Namespace: "default",
		Proposal:  workbenchView{Proposal: "proposal"},
		Selected:  &workbenchSelectedTarget{Target: target, Projection: dtoProjection{}},
	}
	var body bytes.Buffer
	if err := workbenchClusterPageTemplate.Execute(&body, view); err != nil {
		t.Fatalf("cluster template execution = %v", err)
	}
	text := body.String()
	if !strings.Contains(text, "Runtime subject / provenance") {
		t.Fatal("selected view omitted the runtime subject/provenance panel entirely")
	}
	if !strings.Contains(text, "NOT_AVAILABLE — no current runtime incarnation was discovered for this target") {
		t.Errorf("absent runtime subjects did not render an explicit NOT_AVAILABLE state:\n%s", text)
	}
}

func TestWorkbenchTargetLinkUsesOnlyCanonicalTargetFields(t *testing.T) {
	link := workbenchTargetLink(k8s.GovernedTarget{Namespace: "default", Workload: k8s.WorkloadRef{Group: "apps", Kind: "Deployment", Name: "api"}, Container: "web"})
	if link != "?container=web&group=apps&kind=Deployment&name=api" {
		t.Fatalf("target link = %q", link)
	}
}

func TestShellQuoteRendersOneLiteralPOSIXWord(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "simple", value: "my-proposal", want: "'my-proposal'"},
		{name: "empty", value: "", want: "''"},
		{name: "space", value: "hello world", want: "'hello world'"},
		{name: "single quote", value: "foo'bar", want: "'foo'\"'\"'bar'"},
		{name: "double quote", value: `foo"bar`, want: `'foo"bar'`},
		{name: "semicolon", value: "foo;id", want: "'foo;id'"},
		{name: "command substitution", value: "$(id)", want: "'$(id)'"},
		{name: "backticks", value: "`id`", want: "'`id`'"},
		{name: "variable expansion", value: "$HOME", want: "'$HOME'"},
		{name: "newline", value: "foo\nid", want: "'foo\nid'"},
		{name: "tab", value: "foo\tbar", want: "'foo\tbar'"},
		{name: "glob", value: "*", want: "'*'"},
		{name: "backslash", value: `foo\bar`, want: `'foo\bar'`},
		{name: "mixed adversarial", value: "$(id);`uname` $HOME * foo'bar\n", want: "'$(id);`uname` $HOME * foo'\"'\"'bar\n'"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shellQuote(test.value); got != test.want {
				t.Fatalf("shellQuote(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestWorkbenchNextStepsShellQuoteDynamicArguments(t *testing.T) {
	steps := workbenchNextSteps("foo;id", "hello world", "sha256:$(id)", "NOT BOUND")
	wantApprove := `kubectl landlock-genprof approve 'foo;id' -n 'hello world' --expected-digest 'sha256:$(id)'`
	wantApply := `kubectl landlock-genprof apply-proposal 'foo;id' -n 'hello world'`
	if !reflect.DeepEqual(steps, []string{wantApprove, wantApply}) {
		t.Fatalf("next steps = %#v, want %#v", steps, []string{wantApprove, wantApply})
	}
	if bound := workbenchNextSteps("proposal", "default", "sha256:abc", "BOUND — approved digest validates against the current candidate"); !reflect.DeepEqual(bound, []string{"kubectl landlock-genprof apply-proposal 'proposal' -n 'default'"}) {
		t.Fatalf("bound next steps = %#v", bound)
	}
}

// --- DTO losslessness proof (§11) ---
//
// These build projection.WorkloadSecurityProjection values directly —
// bypassing HTTP and the cluster entirely — so the conversion function
// itself is what is under test, independent of any particular fixture path.

func TestProjectionDTO_EmptyStateSurvives(t *testing.T) {
	dto := dtoFromProjection(projection.WorkloadSecurityProjection{
		Runtime: projection.RuntimeEvidence{Section: projection.Section{State: projection.Empty, Reason: "no evidence supplied"}},
	})
	if dto.Runtime.State != string(projection.Empty) || dto.Runtime.Reason != "no evidence supplied" {
		t.Fatalf("Runtime = %+v", dto.Runtime)
	}
	if len(dto.Runtime.Evidence) != 0 || len(dto.Runtime.Excluded) != 0 {
		t.Fatalf("EMPTY state must carry no observations: %+v", dto.Runtime)
	}
}

func TestProjectionDTO_AvailableStateSurvives(t *testing.T) {
	target := k8s.GovernedTarget{Namespace: "default", Workload: k8s.WorkloadRef{Kind: "Pod", Name: "p"}, Container: "app"}
	dto := dtoFromProjection(projection.WorkloadSecurityProjection{
		Target: target,
		Runtime: projection.RuntimeEvidence{
			Section: projection.Section{State: projection.Available, Reason: "1 attributed"},
			Evidence: []projection.Evidence{{
				Source:      association.Evidence{Target: &target, Population: history.Population{Container: "app", ImageIdentity: "sha256:a", BinaryPath: "/app"}},
				Association: association.Result{State: association.Associated, Reason: "exact match"},
			}},
		},
	})
	if dto.Runtime.State != string(projection.Available) {
		t.Fatalf("state = %q", dto.Runtime.State)
	}
	if len(dto.Runtime.Evidence) != 1 {
		t.Fatalf("evidence lost: %+v", dto.Runtime)
	}
	got := dto.Runtime.Evidence[0]
	if got.Association.State != string(association.Associated) || got.Association.Reason != "exact match" {
		t.Errorf("association state/reason lost: %+v", got.Association)
	}
	if got.Source.ImageIdentity != "sha256:a" || got.Source.BinaryPath != "/app" {
		t.Errorf("evidence source population lost: %+v", got.Source)
	}
	if got.Source.Target == nil || got.Source.Target.Workload.Name != "p" {
		t.Errorf("evidence source target lost: %+v", got.Source.Target)
	}
}

func TestProjectionDTO_UnknownMixedRuntimePreservesAttributedAndExcluded(t *testing.T) {
	target := k8s.GovernedTarget{Namespace: "default", Workload: k8s.WorkloadRef{Kind: "Pod", Name: "p"}, Container: "app"}
	other := k8s.GovernedTarget{Namespace: "default", Workload: k8s.WorkloadRef{Kind: "Pod", Name: "other"}, Container: "app"}
	dto := dtoFromProjection(projection.WorkloadSecurityProjection{
		Runtime: projection.RuntimeEvidence{
			Section: projection.Section{State: projection.Unknown, Reason: "1 attributed and 1 excluded"},
			Evidence: []projection.Evidence{{
				Source:      association.Evidence{Target: &target},
				Association: association.Result{State: association.Associated, Reason: "match"},
			}},
			Excluded: []projection.ExcludedEvidence{{
				Source:      association.Evidence{Target: &other},
				Association: association.Result{State: association.Unassociated, Reason: "different canonical target"},
			}},
		},
	})
	if dto.Runtime.State != string(projection.Unknown) {
		t.Fatalf("state = %q, want UNKNOWN", dto.Runtime.State)
	}
	if len(dto.Runtime.Evidence) != 1 {
		t.Fatalf("attributed evidence lost in mixed population: %+v", dto.Runtime)
	}
	if len(dto.Runtime.Excluded) != 1 {
		t.Fatalf("excluded evidence lost in mixed population: %+v", dto.Runtime)
	}
	if dto.Runtime.Excluded[0].Association.State != string(association.Unassociated) {
		t.Errorf("excluded association state lost: %+v", dto.Runtime.Excluded[0])
	}
	if dto.Runtime.Excluded[0].Association.Reason != "different canonical target" {
		t.Errorf("excluded association reason lost: %+v", dto.Runtime.Excluded[0])
	}
}

func TestProjectionDTO_ExcludedGovernanceProposalSurvivesWithExclusionAndAssociation(t *testing.T) {
	assocResult := association.Result{State: association.InsufficientProvenance, Reason: "proposal lacks a complete canonical target"}
	dto := dtoFromProjection(projection.WorkloadSecurityProjection{
		Governance: projection.ProposalGovernance{
			Section: projection.Section{State: projection.Unknown, Reason: "1 excluded"},
			Excluded: []projection.ExcludedProposal{{
				Source:      projection.SourceRef{Kind: "SecurityProfileProposal", Namespace: "default", Name: "legacy"},
				Exclusion:   projection.ProposalNotAssociated,
				Association: &assocResult,
				Reason:      assocResult.Reason,
			}},
		},
	})
	if len(dto.Governance.Proposals) != 0 {
		t.Fatalf("excluded proposal was attributed: %+v", dto.Governance.Proposals)
	}
	if len(dto.Governance.Excluded) != 1 {
		t.Fatalf("excluded proposal lost: %+v", dto.Governance)
	}
	got := dto.Governance.Excluded[0]
	if got.Exclusion != string(projection.ProposalNotAssociated) {
		t.Errorf("exclusion type lost: %+v", got)
	}
	if got.Association == nil || got.Association.State != string(association.InsufficientProvenance) {
		t.Errorf("association state lost: %+v", got.Association)
	}
	if got.Reason != assocResult.Reason {
		t.Errorf("exclusion reason lost: got %q want %q", got.Reason, assocResult.Reason)
	}
}

func TestProjectionDTO_UninterpretedProposalHasNoAssociation(t *testing.T) {
	dto := dtoFromProjection(projection.WorkloadSecurityProjection{
		Governance: projection.ProposalGovernance{
			Excluded: []projection.ExcludedProposal{{
				Source:    projection.SourceRef{Kind: "SecurityProfileProposal", Namespace: "default", Name: "broken"},
				Exclusion: projection.ProposalNotInterpreted,
				Reason:    "proposal spec does not convert to a candidate",
			}},
		},
	})
	if len(dto.Governance.Excluded) != 1 {
		t.Fatalf("excluded proposal lost: %+v", dto.Governance)
	}
	got := dto.Governance.Excluded[0]
	if got.Exclusion != string(projection.ProposalNotInterpreted) {
		t.Errorf("exclusion type lost: %+v", got)
	}
	// Association was never reached — the DTO must not invent one.
	if got.Association != nil {
		t.Errorf("association fabricated for a never-reached association step: %+v", got.Association)
	}
}

func TestProjectionDTO_ApprovalBindingValidityAndReasonSurvive(t *testing.T) {
	dto := dtoFromProjection(projection.WorkloadSecurityProjection{
		Governance: projection.ProposalGovernance{
			Proposals: []projection.ProposalState{{
				Source:                  projection.SourceRef{Kind: "SecurityProfileProposal", Namespace: "default", Name: "p"},
				Association:             association.Result{State: association.Associated},
				CandidateDigest:         "sha256:current",
				ApprovedCandidateDigest: "sha256:stale",
				ApprovalBindingValid:    false,
				ApprovalBindingReason:   "approved candidate digest sha256:stale does not match the current candidate sha256:current",
				ApprovalState:           "Approved",
			}},
		},
	})
	if len(dto.Governance.Proposals) != 1 {
		t.Fatalf("attributed proposal lost: %+v", dto.Governance)
	}
	got := dto.Governance.Proposals[0]
	if got.ApprovalBindingValid {
		t.Error("approval binding validity flipped to true")
	}
	if got.ApprovalBindingReason == "" || got.ApprovedCandidateDigest != "sha256:stale" || got.CandidateDigest != "sha256:current" {
		t.Errorf("approval binding provenance lost: %+v", got)
	}
}

func TestProjectionDTO_EnforcementAndBehavioralVerificationAreNestedNotFlattened(t *testing.T) {
	dto := dtoFromProjection(projection.WorkloadSecurityProjection{
		Enforcement:            projection.EnforcementEvidence{Section: projection.Section{State: projection.NotAvailable, Reason: "no target-bound enforcement proof is persisted"}},
		BehavioralVerification: projection.BehavioralVerification{Section: projection.Section{State: projection.NotAvailable, Reason: "no target-bound behavioral verification is persisted"}},
	})
	encoded, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &generic); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	for _, key := range []string{"enforcement", "behavioralVerification"} {
		raw, ok := generic[key]
		if !ok {
			t.Fatalf("top-level projection JSON has no %q field: %s", key, encoded)
		}
		var section map[string]json.RawMessage
		if err := json.Unmarshal(raw, &section); err != nil {
			t.Fatalf("%q is not a nested object: %s", key, raw)
		}
		if _, ok := section["state"]; !ok {
			t.Errorf("%q object has no nested state field: %s", key, raw)
		}
	}
	// Guard against accidental Section embedding flattening these into the
	// top-level object, which is exactly the Q2 defect this DTO exists to
	// avoid.
	if _, ok := generic["state"]; ok {
		t.Error("a top-level \"state\" field leaked from an embedded Section — Enforcement/BehavioralVerification flattened accidentally")
	}
}

func discoveryResultFixture(target k8s.GovernedTarget, subject k8s.RuntimeSubject) workload.Result {
	return workload.Result{
		State:     workload.StateReady,
		Namespace: target.Namespace,
		Workloads: []workload.Workload{{
			Target: target.Workload,
			Owner:  workload.OwnerBarePod,
			Pods: []workload.Pod{{
				Name: target.Workload.Name,
				Containers: []workload.Container{{
					Name: target.Container, Category: workload.ContainerRegular, SupportedTarget: true,
					Target: &target, Runtime: &subject, RuntimeState: workload.RuntimeAvailable,
				}},
			}},
		}},
	}
}

func TestDiscoveryDTO_PreservesContainerTargetAndRuntimeSubject(t *testing.T) {
	target := k8s.GovernedTarget{Namespace: "default", Workload: k8s.WorkloadRef{Kind: "Pod", Name: "p"}, Container: "app"}
	subject := k8s.RuntimeSubject{Target: target, PodUID: "uid-1", ImageID: "sha256:img", BinaryPath: "/app"}
	dto := dtoFromDiscoveryResult(discoveryResultFixture(target, subject))
	if len(dto.Workloads) != 1 || len(dto.Workloads[0].Pods) != 1 || len(dto.Workloads[0].Pods[0].Containers) != 1 {
		t.Fatalf("discovery shape lost: %+v", dto)
	}
	container := dto.Workloads[0].Pods[0].Containers[0]
	if container.Target == nil || container.Target.Workload.Name != "p" {
		t.Fatalf("Container.Target lost: %+v", container)
	}
	if container.Runtime == nil || container.Runtime.PodUID != "uid-1" || container.Runtime.ImageID != "sha256:img" {
		t.Fatalf("Container.Runtime lost: %+v", container)
	}
}
