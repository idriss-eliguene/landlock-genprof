// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/yaml"

	"github.com/idriss-eliguene/landlock-genprof/internal/analysis"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/observation"
	"github.com/idriss-eliguene/landlock-genprof/internal/policy"
	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	"github.com/idriss-eliguene/landlock-genprof/internal/semantic"
	adpt "github.com/idriss-eliguene/landlock-genprof/internal/semantic/adapter"
	"github.com/idriss-eliguene/landlock-genprof/internal/spobackend"
	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
	"reflect"
	"time"
)

func TestProcessTraceEvents_FilesystemOnly(t *testing.T) {
	now := time.Now().UTC()
	events := []tracer.Event{{
		Timestamp: now,
		Path:      "/etc/passwd",
		Mode:      "read",
	}}
	source := semantic.NewSubjectIdentity("landlock-genprof")
	runMeta := adpt.RunMeta{Source: source, Start: nil, End: nil, RecordTime: now}
	behavior, br, err := processTraceEvents(context.Background(), events, runMeta, nil)
	if err != nil {
		t.Fatalf("processTraceEvents error = %v", err)
	}
	// policy.Synthesize on same events should match returned behavior
	observations := make([]observation.Observation, 0, len(events))
	for _, ev := range events {
		observations = append(observations, tracer.ToObservation(ev))
	}
	wantBehavior, err := policy.Synthesize(observations, nil)
	if err != nil {
		t.Fatalf("policy.Synthesize error = %v", err)
	}
	if got, want := behavior.Filesystem, wantBehavior.Filesystem; !reflect.DeepEqual(got, want) {
		t.Fatalf("Filesystem profile mismatch: got=%#v want=%#v", got, want)
	}
	if br == nil {
		t.Fatal("semantic BuildResult is nil")
	}
	acts := br.Graph.GetActs()
	if len(acts) != 1 {
		t.Fatalf("expected 1 Act, got %d", len(acts))
	}
	if acts[0].Source() != source {
		t.Fatalf("Act Source = %q, want %q", acts[0].Source(), source)
	}
}

func TestProcessTraceEvents_MixedStreamPartitioning(t *testing.T) {
	now := time.Now().UTC()
	events := []tracer.Event{
		{Timestamp: now, Path: "/etc/hosts", Mode: "read"},
		{Timestamp: now, Syscall: "connect", Mode: "egress", Port: 80},
	}
	source := semantic.NewSubjectIdentity("landlock-genprof")
	runMeta := adpt.RunMeta{Source: source, Start: nil, End: nil, RecordTime: now}
	behavior, br, err := processTraceEvents(context.Background(), events, runMeta, nil)
	if err != nil {
		t.Fatalf("processTraceEvents error = %v", err)
	}
	// ensure policy still sees network access
	if len(behavior.Network.Accesses) == 0 {
		t.Fatalf("expected policy to see network accesses, got none")
	}
	// adapter result must exist and include only filesystem events
	if br == nil {
		t.Fatal("semantic BuildResult is nil")
	}
	evidenceGroups := 0
	for _, g := range br.EvidenceGroups {
		evidenceGroups += len(g)
	}
	// adapter now ingests both filesystem and network observations, expecting 2 evidence entries
	if evidenceGroups != 2 {
		t.Fatalf("expected adapter evidence groups size 2, got %d", evidenceGroups)
	}
}

func TestProcessTraceEvents_1vs100Dedup(t *testing.T) {
	now := time.Now().UTC()
	e := tracer.Event{Timestamp: now, Path: "/tmp/x", Mode: "write"}
	many := make([]tracer.Event, 100)
	for i := 0; i < 100; i++ {
		many[i] = e
	}
	source := semantic.NewSubjectIdentity("landlock-genprof")
	runMeta1 := adpt.RunMeta{Source: source, Start: nil, End: nil, RecordTime: now}
	b1, br1, err := processTraceEvents(context.Background(), []tracer.Event{e}, runMeta1, nil)
	if err != nil {
		t.Fatalf("processTraceEvents single error = %v", err)
	}
	runMeta100 := adpt.RunMeta{Source: source, Start: nil, End: nil, RecordTime: now}
	b100, br100, err := processTraceEvents(context.Background(), many, runMeta100, nil)
	if err != nil {
		t.Fatalf("processTraceEvents many error = %v", err)
	}
	// behavior should match direct policy synthesis for each input
	obs1 := []observation.Observation{tracer.ToObservation(e)}
	wantB1, err := policy.Synthesize(obs1, nil)
	if err != nil {
		t.Fatalf("policy.Synthesize single error = %v", err)
	}
	if !reflect.DeepEqual(b1.Filesystem, wantB1.Filesystem) {
		t.Fatalf("BehaviorProfile mismatch for single: got=%v want=%v", b1.Filesystem, wantB1.Filesystem)
	}
	manyObs := make([]observation.Observation, 0, len(many))
	for _, ev := range many {
		manyObs = append(manyObs, tracer.ToObservation(ev))
	}
	wantB100, err := policy.Synthesize(manyObs, nil)
	if err != nil {
		t.Fatalf("policy.Synthesize many error = %v", err)
	}
	if !reflect.DeepEqual(b100.Filesystem, wantB100.Filesystem) {
		t.Fatalf("BehaviorProfile mismatch for many: got=%v want=%v", b100.Filesystem, wantB100.Filesystem)
	}
	if len(br1.AssertionIDs) != 1 || len(br100.AssertionIDs) != 1 {
		t.Fatalf("expected single assertion id for both cases, got %d and %d", len(br1.AssertionIDs), len(br100.AssertionIDs))
	}
	if len(br100.EvidenceGroups[br100.AssertionIDs[0]]) != 100 {
		t.Fatalf("expected 100 evidence entries, got %d", len(br100.EvidenceGroups[br100.AssertionIDs[0]]))
	}
}

func TestProcessTraceEvents_RelativePathExcluded(t *testing.T) {
	now := time.Now().UTC()
	events := []tracer.Event{{Timestamp: now, Path: "relative/path", Mode: "read"}}
	source := semantic.NewSubjectIdentity("landlock-genprof")
	runMeta := adpt.RunMeta{Source: source, Start: nil, End: nil, RecordTime: now}
	_, br, err := processTraceEvents(context.Background(), events, runMeta, nil)
	if err != nil {
		t.Fatalf("processTraceEvents error = %v", err)
	}
	// adapter should have zero assertion ids
	if br != nil && len(br.AssertionIDs) != 0 {
		t.Fatalf("expected adapter no assertions for relative path, got %d", len(br.AssertionIDs))
	}
}

func TestProcessTraceEvents_ZeroTimestampDiagnosticIncludesContext(t *testing.T) {
	now := time.Now().UTC()
	events := []tracer.Event{
		{
			Timestamp: time.Time{},
			Syscall:   "connect",
			Mode:      "egress",
			Port:      8080,
			Provenance: &tracer.ProvenanceDescriptor{
				BackendKind: "trace_tcp",
				OriginType:  "direct",
			},
			TimestampDiag: &tracer.TimestampExtractionDiagnostic{
				DataSource:    "tracetcp",
				Field:         "timestamp_raw",
				AccessorError: "invalid field length, expected 8, got 0",
				RawLen:        0,
				RawHex:        "",
			},
		},
	}
	source := semantic.NewSubjectIdentity("landlock-genprof")
	runMeta := adpt.RunMeta{Source: source, RecordTime: now}
	_, _, err := processTraceEvents(context.Background(), events, runMeta, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{
		"invalid event timestamp: zero value present at index 0",
		`kind=network`,
		`datasource="tracetcp"`,
		`field="timestamp_raw"`,
		`accessor_error="invalid field length, expected 8, got 0"`,
		`backend="trace_tcp"`,
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q does not contain %q", msg, want)
		}
	}
}

func TestProcessTraceEvents_ZeroTimestampSyscallStillAccepted(t *testing.T) {
	now := time.Now().UTC()
	events := []tracer.Event{
		{Timestamp: time.Time{}, Syscall: "openat", Mode: "syscall"},
	}
	source := semantic.NewSubjectIdentity("landlock-genprof")
	runMeta := adpt.RunMeta{Source: source, RecordTime: now}
	_, _, err := processTraceEvents(context.Background(), events, runMeta, nil)
	if err != nil {
		t.Fatalf("expected syscall zero timestamp to be accepted, got error: %v", err)
	}
}

func TestAddPodLockProfileLabel_PodManifest(t *testing.T) {
	in := []byte(`apiVersion: v1
kind: Pod
metadata:
  name: nginx-demo
  namespace: default
spec:
  containers:
    - name: nginx
      image: nginx:alpine
`)

	out, err := addPodLockProfileLabel(in, "nginx-demo")
	if err != nil {
		t.Fatalf("addPodLockProfileLabel() error = %v", err)
	}

	var obj map[string]interface{}
	if err := yaml.Unmarshal(out, &obj); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	labels, found, err := unstructured.NestedStringMap(obj, "metadata", "labels")
	if err != nil {
		t.Fatalf("NestedStringMap(metadata.labels) error = %v", err)
	}
	if !found {
		t.Fatal("metadata.labels not found")
	}
	if labels[podLockProfileLabel] != "nginx-demo" {
		t.Fatalf("metadata.labels[%q] = %q, want nginx-demo", podLockProfileLabel, labels[podLockProfileLabel])
	}
}

func TestAddPodLockProfileLabel_DeploymentManifest(t *testing.T) {

	in := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-demo
  namespace: default
spec:
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
        - name: nginx
          image: nginx:alpine
`)

	out, err := addPodLockProfileLabel(in, "nginx-demo")
	if err != nil {
		t.Fatalf("addPodLockProfileLabel() error = %v", err)
	}

	var obj map[string]interface{}
	if err := yaml.Unmarshal(out, &obj); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	labels, found, err := unstructured.NestedStringMap(obj, "spec", "template", "metadata", "labels")
	if err != nil {
		t.Fatalf("NestedStringMap(spec.template.metadata.labels) error = %v", err)
	}
	if !found {
		t.Fatal("spec.template.metadata.labels not found")
	}
	if labels[podLockProfileLabel] != "nginx-demo" {
		t.Fatalf("template labels[%q] = %q, want nginx-demo", podLockProfileLabel, labels[podLockProfileLabel])
	}
}

func TestPrintSecurityRecommendationSummary(t *testing.T) {
	rec := analysis.SecurityRecommendation{
		Workload:          analysis.WorkloadRef{Namespace: "default", Pod: "payment-api", Container: "app"},
		TrainingRuns:      14,
		OverallConfidence: 94,
		Domains: []analysis.DomainRecommendation{
			{Domain: "filesystem", RequiredCount: 23, Backend: "podlock", Available: true},
			{Domain: "network", RequiredCount: 4, Backend: "networkpolicy", Available: true},
		},
	}

	var out bytes.Buffer
	printSecurityRecommendationSummary(&out, rec)
	got := out.String()

	for _, want := range []string{
		"WORKLOAD SECURITY ANALYSIS",
		"Workload: default/payment-api",
		"Training runs: 14",
		"filesystem: 23 item(s) -> podlock",
		"Overall confidence: 94%",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary missing %q, got: %s", want, got)
		}
	}
}

func TestPublishProposal_SavesMandatoryProposal(t *testing.T) {
	dynClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	oldFactory := newDynamicClientForProposal
	newDynamicClientForProposal = func() (dynamic.Interface, error) { return dynClient, nil }
	defer func() { newDynamicClientForProposal = oldFactory }()

	target := &k8s.TargetPod{
		Namespace: "default",
		PodName:   "nginx-demo",
		Container: "nginx",
		Labels:    map[string]string{"app": "nginx"},
	}

	behavior := profile.BehaviorProfile{
		Filesystem: profile.FilesystemProfile{Accesses: []profile.FileAccess{{
			Path:        "/etc/nginx",
			Permissions: []profile.FilePermission{profile.PermissionRead},
			Confidence:  profile.ConfidenceHigh,
			SeenCount:   2,
		}}},
	}

	var stdout bytes.Buffer
	err := publishProposal(
		context.Background(),
		&stdout,
		k8sfake.NewSimpleClientset(&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "nginx-demo", Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx:test"}}},
		}),
		target,
		target,
		k8s.OwnerNone,
		traceOptions{binary: "/usr/sbin/nginx", history: true},
		behavior,
		seccompSource{kind: spobackend.SeccompSourceInternal, provenance: spobackend.InternalSeccompProvenance()},
		"",
	)
	if err != nil {
		t.Fatalf("publishProposal() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "SecurityProfileProposal published: nginx-demo") {
		t.Fatalf("publishProposal() did not report publication, stdout = %q", stdout.String())
	}

	got, err := proposal.Get(context.Background(), dynClient, "default", "nginx-demo")
	if err != nil {
		t.Fatalf("proposal.Get() error = %v", err)
	}
	if got == nil {
		t.Fatal("proposal.Get() = nil, want stored proposal")
	}
	if got.Container != "nginx" {
		t.Fatalf("proposal.Container = %q, want nginx", got.Container)
	}
	if got.Binary != "/usr/sbin/nginx" {
		t.Fatalf("proposal.Binary = %q, want /usr/sbin/nginx", got.Binary)
	}
	if !got.HistoryUsed {
		t.Fatal("proposal.HistoryUsed = false, want true")
	}
	if got.GeneratedAt == "" {
		t.Fatal("proposal.GeneratedAt = empty, want RFC3339 timestamp")
	}
	if !strings.Contains(got.PodLock, "kind: LandlockProfile") {
		t.Fatalf("proposal.PodLock missing LandlockProfile YAML, got %q", got.PodLock)
	}
	if got.NetworkPolicy != "" {
		t.Fatalf("proposal.NetworkPolicy = %q, want empty (no network accesses)", got.NetworkPolicy)
	}
	if got.PatchedManifest == "" {
		t.Fatal("proposal.PatchedManifest is empty, want PodLock binding manifest")
	}
	if got.SPOSeccompProfile != "" {
		t.Fatalf("proposal.SPOSeccompProfile = %q, want empty (no syscalls)", got.SPOSeccompProfile)
	}
}

func TestPublishProposal_PodLockOnlyIncludesBindingManifest(t *testing.T) {
	dynClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	oldFactory := newDynamicClientForProposal
	newDynamicClientForProposal = func() (dynamic.Interface, error) { return dynClient, nil }
	defer func() { newDynamicClientForProposal = oldFactory }()

	target := &k8s.TargetPod{Namespace: "podlock-test", PodName: "profile-realization", Container: "probe"}
	client := k8sfake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: target.PodName, Namespace: target.Namespace},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "probe", Image: "probe:test"}}},
	})
	behavior := profile.BehaviorProfile{Filesystem: profile.FilesystemProfile{Accesses: []profile.FileAccess{{
		Path: "/data/allowed.txt", Permissions: []profile.FilePermission{profile.PermissionRead},
	}}}}

	var stdout bytes.Buffer
	if err := publishProposal(context.Background(), &stdout, client, target, target, k8s.OwnerNone,
		traceOptions{binary: "/probe/fsprobe"}, behavior,
		seccompSource{kind: spobackend.SeccompSourceInternal}, ""); err != nil {
		t.Fatalf("publishProposal() error = %v", err)
	}
	got, err := proposal.Get(context.Background(), dynClient, target.Namespace, target.PodName)
	if err != nil {
		t.Fatalf("proposal.Get() error = %v", err)
	}
	if got.PodLock == "" || got.PatchedManifest == "" {
		t.Fatalf("PodLock-only proposal missing artifact: podlock=%t patched=%t", got.PodLock != "", got.PatchedManifest != "")
	}
	var manifest unstructured.Unstructured
	if err := yaml.Unmarshal([]byte(got.PatchedManifest), &manifest.Object); err != nil {
		t.Fatalf("parsing patched manifest: %v", err)
	}
	labels, _, err := unstructured.NestedStringMap(manifest.Object, "metadata", "labels")
	if err != nil || labels[podLockProfileLabel] != target.PodName {
		t.Fatalf("PodLock label = %q, want %q", labels[podLockProfileLabel], target.PodName)
	}
	containers, found, err := unstructured.NestedSlice(manifest.Object, "spec", "containers")
	if err != nil || !found || len(containers) != 1 {
		t.Fatalf("patched manifest containers = %#v, want one container", containers)
	}
	container, ok := containers[0].(map[string]interface{})
	if !ok {
		t.Fatalf("patched manifest container has type %T", containers[0])
	}
	if _, found := container["securityContext"]; found {
		t.Fatalf("PodLock-only manifest manufactured a securityContext")
	}
	var profileObj unstructured.Unstructured
	if err := yaml.Unmarshal([]byte(got.PodLock), &profileObj.Object); err != nil {
		t.Fatalf("parsing PodLock artifact: %v", err)
	}
	profileName, _, _ := unstructured.NestedString(profileObj.Object, "metadata", "name")
	if labels[podLockProfileLabel] != profileName {
		t.Fatalf("binding label %q does not reference profile %q", labels[podLockProfileLabel], profileName)
	}
}

func TestPodLockBindingParticipatesInCandidateDigest(t *testing.T) {
	base := proposal.Spec{Container: "probe", Binary: "/probe/fsprobe", PodLock: "profile"}
	withBinding := base
	withBinding.PatchedManifest = "metadata:\n  labels:\n    podlock.kubewarden.io/profile: profile\n"
	mutated := withBinding
	mutated.PatchedManifest = "metadata:\n  labels:\n    podlock.kubewarden.io/profile: other\n"
	d1, err := proposal.CandidateDigest(withBinding)
	if err != nil {
		t.Fatalf("CandidateDigest() error = %v", err)
	}
	d2, err := proposal.CandidateDigest(mutated)
	if err != nil {
		t.Fatalf("CandidateDigest() error = %v", err)
	}
	if d1 == d2 {
		t.Fatalf("binding mutation did not change CandidateDigest: %s", d1)
	}
}
