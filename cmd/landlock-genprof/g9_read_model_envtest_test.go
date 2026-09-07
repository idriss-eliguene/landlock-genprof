//go:build envtest

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	obsdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	obskube "github.com/idriss-eliguene/landlock-genprof/internal/observation/kubernetes"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

func g9ReadObservation(t *testing.T, id, uid, name, container, image string) obsdomain.Observation {
	t.Helper()
	cluster, err := obsdomain.NewClusterIdentity("cluster-uid")
	if err != nil {
		t.Fatal(err)
	}
	workload := obsdomain.WorkloadIdentity{Cluster: cluster, Namespace: "default", GroupKind: obsdomain.GroupKind{Group: "apps", Kind: "Deployment"}, Name: name, UID: uid}
	slot := obsdomain.ContainerSlot{Workload: workload, Container: container}
	spec, err := obsdomain.NewObservationSpec(obsdomain.RequestedTarget{Slot: slot}, []string{"capabilities", "exec"}, time.Minute, "g9")
	if err != nil {
		t.Fatal(err)
	}
	o, err := obsdomain.NewObservation(obsdomain.ObservationID(id), spec)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := obsdomain.NewContainerImageRevision(slot, image)
	if err != nil {
		t.Fatal(err)
	}
	runtimeInstance := obsdomain.RuntimeContainerInstance{Slot: slot, PodUID: "pod-" + id, ImageRevision: &revision}
	resolved, err := obsdomain.NewResolvedTargetSet([]obsdomain.RuntimeContainerInstance{runtimeInstance})
	if err != nil {
		t.Fatal(err)
	}
	if err := o.Bind(resolved, obsdomain.BackendIdentity{Kind: "fixture", Version: "v1"}, []obsdomain.ContainerImageRevision{revision}); err != nil {
		t.Fatal(err)
	}
	q := obsdomain.SourceQualification{BackendHealthConfirmed: true, SourceAttachedForBoundWindow: true, FlushConfirmed: true, Attribution: obsdomain.AttributionCompleted, AttributedCount: 1}
	source, err := obsdomain.NewSourceResult(obsdomain.EvidenceSource{Name: "capabilities", Backend: "fixture", Version: "v1"}, q, nil, obsdomain.NormalizedFacts{Capabilities: []obsdomain.CapabilityFact{{Name: "CAP_CHOWN"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := o.RecordSourceResult(source); err != nil {
		t.Fatal(err)
	}
	return o
}

func g9ReadServer(t *testing.T) (*workbenchServer, dynamic.Interface) {
	t.Helper()
	dyn := e2eDynamicClient(t)
	reads, err := k8s.NewReadSession(e2eConfig, "default")
	if err != nil {
		t.Fatal(err)
	}
	s, err := newWorkbenchServer(reads, "", 18082)
	if err != nil {
		t.Fatal(err)
	}
	s.allowedHost = realObservationHost
	s.allowedOrigin = "http://" + realObservationHost
	return s, dyn
}

func TestG9ReadModelRealEnvtestMatrices(t *testing.T) {
	server, dyn := g9ReadServer(t)
	image := "sha256:" + strings.Repeat("a", 64)
	first := g9ReadObservation(t, "g9-rm-observation", "workload-uid", "api", "app", image)
	second := g9ReadObservation(t, "g9-rm-observation-2", "workload-uid", "api", "app", image)
	seedRealObservation(t, dyn, first)
	seedRealObservation(t, dyn, second)
	otherUID := g9ReadObservation(t, "g9-rm-other-uid", "other-uid", "api", "app", image)
	seedRealObservation(t, dyn, otherUID)
	otherContainer := g9ReadObservation(t, "g9-rm-other-container", "workload-uid", "api", "sidecar", image)
	seedRealObservation(t, dyn, otherContainer)

	q := "?group=apps&kind=Deployment&name=api&container=app&workloadUID=workload-uid"
	resp := realObservationRequest(t, server, http.MethodGet, "/api/observations"+q, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("O1 list status=%d body=%s", resp.Code, resp.Body.String())
	}
	var list struct {
		Items []observationRead `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 2 || list.Items[0].ID >= list.Items[1].ID {
		t.Fatalf("O2/O7/O8 list=%#v", list.Items)
	}
	if list.Items[0].Identity.WorkloadUID != "workload-uid" {
		t.Fatalf("UID-1 identity=%#v", list.Items[0].Identity)
	}
	for _, item := range list.Items {
		if item.Identity.WorkloadUID != "workload-uid" || item.Identity.Container != "app" {
			t.Fatalf("O3/O5 contamination=%#v", item)
		}
	}
	detail := realObservationRequest(t, server, http.MethodGet, "/api/observations/"+string(first.ID()), nil)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "CAP_CHOWN") {
		t.Fatalf("O9/O10/O13 detail status=%d body=%s", detail.Code, detail.Body.String())
	}
	missing := realObservationRequest(t, server, http.MethodGet, "/api/observations/missing", nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("O14 status=%d", missing.Code)
	}
	// A new request with no client-side state rediscovers the same durable items.
	again := realObservationRequest(t, server, http.MethodGet, "/api/observations"+q, nil)
	if again.Code != http.StatusOK {
		t.Fatalf("O15 status=%d", again.Code)
	}

	spec := proposal.Spec{CandidateVersion: proposal.CandidateVersionV2, GeneratedAt: "2026-09-07T00:00:00Z", Subject: &proposal.SubjectV2{Scope: proposal.CandidateV2ScopeContainer, Target: "Deployment/api", Container: "app", ImageIdentity: image}, CapabilityArtifact: &proposal.ArtifactV2{Type: proposal.CandidateV2ArtifactContainerCaps, ContainerCapabilities: proposal.ContainerCapabilitiesV2{Drop: []string{"ALL"}, Add: []string{"CAP_CHOWN"}}}, Provenance: &proposal.ProposalProvenance{PopulationScope: proposal.CandidateV2ScopeContainer, ObservationIDs: []string{string(first.ID())}}, Qualification: &proposal.ProposalQualification{Filesystem: "EMPTY", Exec: "UNKNOWN", NetworkConnect: "EMPTY", NetworkBind: "EMPTY", Capabilities: "AVAILABLE"}, DerivationStatus: &proposal.ProposalDerivationStatus{Capabilities: "SUPPORTED", PodLock: "UNSUPPORTED", NetworkPolicy: "NOT_AVAILABLE", Seccomp: "UNSUPPORTED"}}
	if err := proposal.Save(context.Background(), dyn, "default", "g9-rm-proposal", spec); err != nil {
		t.Fatal(err)
	}
	pr := realObservationRequest(t, server, http.MethodGet, "/api/proposals/g9-rm-proposal", nil)
	if pr.Code != http.StatusOK {
		t.Fatalf("P1/P18 status=%d body=%s", pr.Code, pr.Body.String())
	}
	var got proposalRead
	if err := json.Unmarshal(pr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Subject == nil || got.Subject.Scope != proposal.CandidateV2ScopeContainer || got.Subject.Container != "app" || got.Artifact == nil || got.Artifact.Type != proposal.CandidateV2ArtifactContainerCaps || got.CandidateDigest == "" || got.ReviewContextDigest == "" {
		t.Fatalf("P6-P9 projection=%#v", got)
	}
	if got.UID == "" || got.Status.ApprovalState != proposal.ApprovalDraft || got.CurrentAuthority != "NOT_APPROVED" || got.Provenance == nil || got.Qualification == nil || got.DerivationStatus == nil {
		t.Fatalf("P10-P16/UID-2 projection=%#v", got)
	}
	if got.Subject != nil && strings.Contains(pr.Body.String(), "workloadUID") {
		t.Fatal("UID-2/UID-5 fabricated workload UID in Proposal projection")
	}
	missingProposal := realObservationRequest(t, server, http.MethodGet, "/api/proposals/missing", nil)
	if missingProposal.Code != http.StatusNotFound {
		t.Fatalf("P18 status=%d", missingProposal.Code)
	}
}

var g9ReadProposalGVR = schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "securityprofileproposals"}
var obsGVR = schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "observations"}

func g9ReadProposalSpec(target, container, image string, add string) proposal.Spec {
	return proposal.Spec{CandidateVersion: proposal.CandidateVersionV2, GeneratedAt: "2026-09-07T00:00:00Z", Subject: &proposal.SubjectV2{Scope: proposal.CandidateV2ScopeContainer, Target: target, Container: container, ImageIdentity: image}, CapabilityArtifact: &proposal.ArtifactV2{Type: proposal.CandidateV2ArtifactContainerCaps, ContainerCapabilities: proposal.ContainerCapabilitiesV2{Drop: []string{"ALL"}, Add: []string{add}}}, Provenance: &proposal.ProposalProvenance{PopulationScope: proposal.CandidateV2ScopeContainer, ObservationIDs: []string{"rm-observation"}}, Qualification: &proposal.ProposalQualification{Filesystem: "EMPTY", Exec: "UNKNOWN", NetworkConnect: "EMPTY", NetworkBind: "EMPTY", Capabilities: "AVAILABLE"}, DerivationStatus: &proposal.ProposalDerivationStatus{Capabilities: "SUPPORTED", PodLock: "UNSUPPORTED", NetworkPolicy: "NOT_AVAILABLE", Seccomp: "UNSUPPORTED"}}
}

func TestG9RM1MalformedObservationFailsClosedRealHTTP(t *testing.T) {
	server, dyn := g9ReadServer(t)
	o := g9ReadObservation(t, "rm-malformed-observation", "rm-uid", "api", "app", "sha256:"+strings.Repeat("b", 64))
	raw, err := obskube.ToUnstructured(o, "default")
	if err != nil {
		t.Fatal(err)
	}
	spec, found, err := unstructured.NestedMap(raw.Object, "spec")
	if !found || err != nil {
		t.Fatal("malformed fixture has no spec")
	}
	spec["duration"] = "not-a-duration"
	raw.Object["spec"] = spec
	if _, err = dyn.Resource(obsGVR).Namespace("default").Create(context.Background(), raw, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	resp := realObservationRequest(t, server, http.MethodGet, "/api/observations/"+string(o.ID()), nil)
	if resp.Code == http.StatusOK || resp.Code == http.StatusNotFound {
		t.Fatalf("malformed Observation status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestG9RM2ProposalSelectorExclusionRealHTTP(t *testing.T) {
	server, dyn := g9ReadServer(t)
	image := "sha256:" + strings.Repeat("c", 64)
	matching := g9ReadProposalSpec("Deployment/api", "app", image, "CAP_CHOWN")
	wrongTarget := g9ReadProposalSpec("Deployment/other", "app", image, "CAP_CHOWN")
	wrongContainer := g9ReadProposalSpec("Deployment/api", "sidecar", image, "CAP_CHOWN")
	wrongImage := g9ReadProposalSpec("Deployment/api", "app", "sha256:"+strings.Repeat("d", 64), "CAP_CHOWN")
	for name, spec := range map[string]proposal.Spec{"rm-matching": matching, "rm-wrong-target": wrongTarget, "rm-wrong-container": wrongContainer, "rm-wrong-image": wrongImage} {
		if err := proposal.Save(context.Background(), dyn, "default", name, spec); err != nil {
			t.Fatal(err)
		}
	}
	binaryObj := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "landlockgenprof.io/v1alpha1", "kind": "SecurityProfileProposal", "metadata": map[string]interface{}{"name": "rm-binary", "namespace": "default"}, "spec": map[string]interface{}{"container": "app", "binary": "/bin/app", "generatedAt": "2026-09-07T00:00:00Z"}}}
	if _, err := dyn.Resource(g9ReadProposalGVR).Namespace("default").Create(context.Background(), binaryObj, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	q := "?group=apps&kind=Deployment&name=api&container=app&workloadUID=rm-uid&imageIdentity=" + image
	resp := realObservationRequest(t, server, http.MethodGet, "/api/proposals"+q, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("selector status=%d body=%s", resp.Code, resp.Body.String())
	}
	var list struct {
		Items []proposalRead `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].Name != "rm-matching" {
		t.Fatalf("selector results=%#v", list.Items)
	}
}

func TestG9RM3AuthorityProjectionRealHTTP(t *testing.T) {
	server, dyn := g9ReadServer(t)
	image := "sha256:" + strings.Repeat("e", 64)
	name := "rm-authority"
	spec := g9ReadProposalSpec("Deployment/api", "app", image, "CAP_CHOWN")
	if err := proposal.Save(context.Background(), dyn, "default", name, spec); err != nil {
		t.Fatal(err)
	}
	candidate, err := spec.CandidateV2()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := proposal.CandidateDigestV2(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err := proposal.SetApprovalState(context.Background(), dyn, "default", name, proposal.ApprovalApproved, "rm approval", digest); err != nil {
		t.Fatal(err)
	}
	read := func() proposalRead {
		r := realObservationRequest(t, server, http.MethodGet, "/api/proposals/"+name, nil)
		if r.Code != http.StatusOK {
			t.Fatalf("authority GET status=%d body=%s", r.Code, r.Body.String())
		}
		var p proposalRead
		if err := json.Unmarshal(r.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	approved := read()
	if approved.CurrentAuthority != "VALID" || approved.Status.LastApprovalSnapshot == nil {
		t.Fatalf("approved projection=%#v", approved)
	}
	mutated := g9ReadProposalSpec("Deployment/api", "app", image, "CAP_SETUID")
	if err := proposal.Save(context.Background(), dyn, "default", name, mutated); err != nil {
		t.Fatal(err)
	}
	stale := read()
	if stale.CurrentAuthority != "STALE" || stale.Status.LastApprovalSnapshot == nil || stale.Status.LastApprovalSnapshot.ApprovedCandidateDigest != approved.Status.LastApprovalSnapshot.ApprovedCandidateDigest {
		t.Fatalf("stale projection=%#v", stale)
	}
}

func TestG9RM4MalformedHybridProposalFailsClosedRealHTTP(t *testing.T) {
	server, dyn := g9ReadServer(t)
	image := "sha256:" + strings.Repeat("f", 64)
	spec := g9ReadProposalSpec("Deployment/api", "app", image, "CAP_CHOWN")
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&spec)
	if err != nil {
		t.Fatal(err)
	}
	m["binary"] = "/bin/legacy"
	obj := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "landlockgenprof.io/v1alpha1", "kind": "SecurityProfileProposal", "metadata": map[string]interface{}{"name": "rm-hybrid", "namespace": "default"}, "spec": m}}
	if _, err := dyn.Resource(g9ReadProposalGVR).Namespace("default").Create(context.Background(), obj, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	resp := realObservationRequest(t, server, http.MethodGet, "/api/proposals/rm-hybrid", nil)
	if resp.Code == http.StatusOK || resp.Code == http.StatusNotFound {
		t.Fatalf("hybrid status=%d body=%s", resp.Code, resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), "VALID") {
		t.Fatalf("hybrid claimed authority: %s", resp.Body.String())
	}
}
