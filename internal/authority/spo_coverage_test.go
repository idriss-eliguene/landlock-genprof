package authority

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"

	"github.com/idriss-eliguene/landlock-genprof/internal/spobackend"
	"github.com/idriss-eliguene/landlock-genprof/internal/spoimport"
)

func goldenSPOProfile(coverage string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": spobackend.APIVersion,
		"kind":       spobackend.SeccompProfileKind,
		"metadata": map[string]interface{}{
			"name": "recording-tools", "uid": "6c341716-8ed8-4ee9-84bf-73acdbba1d33", "resourceVersion": "42",
			"labels":      map[string]interface{}{spobackend.RecordingIDLabel: "recording", spobackend.RecordingNamespaceLabel: "recording-ns"},
			"annotations": map[string]interface{}{spobackend.SyscallCoverageAnnotation: coverage},
		},
		"spec": map[string]interface{}{
			"defaultAction": "SCMP_ACT_ERRNO", "architectures": []interface{}{"SCMP_ARCH_X86_64"}, "state": spobackend.SpecStateDisabled,
			"syscalls": []interface{}{map[string]interface{}{"names": []interface{}{"read", "write"}, "action": "SCMP_ACT_ALLOW"}},
		},
	}}
}

func goldenSPODeployment() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{"name": "security-profiles-operator", "namespace": "spo-system"},
		"spec":     map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{"containers": []interface{}{map[string]interface{}{"name": "security-profiles-operator", "image": "spo@sha256:pinned"}}}}},
	}}
}

func observeGoldenSPO(t *testing.T, coverage string) (SPOCoverageObservation, SPOCoverageObservationRequest) {
	t.Helper()
	attempt, scope, contextIdentity, _, _, _ := testInputs(t)
	now := time.Unix(1_800_000_000, 0).UTC()
	in := SPOCoverageObservationRequest{
		Source:  spoimport.Source{Mode: spoimport.ModeMergedProvenance, RecordingName: "recording", RecordingNamespace: "recording-ns", ProfileName: "recording-tools"},
		Target:  spoimport.Target{Namespace: "target-ns", Pod: "target", Container: "tools"},
		Attempt: attempt, Subject: "target-ns/target:tools", Backend: "SECCOMP", Scope: scope, Context: contextIdentity,
		ObservedAt: now, ValidUntil: now.Add(time.Hour), SPOVersion: "305ee9fc8b3128f0ede4459b11f29e09ce61d5ce",
		SPONamespace: "spo-system", SPOImage: "spo@sha256:pinned",
	}
	client := fake.NewSimpleDynamicClient(runtime.NewScheme(), goldenSPOProfile(coverage), goldenSPODeployment())
	observation, err := ObserveSPOCoverage(context.Background(), client, in)
	if err != nil {
		t.Fatal(err)
	}
	return observation, in
}

func TestRealSPOCoverageAuthorityPipeline(t *testing.T) {
	observation, in := observeGoldenSPO(t, `{"version":"v1","total":2,"syscalls":{"read":2,"write":1}}`)
	if got := observation.CoverageSyscalls(); len(got) != 2 || got[0] != "read" || got[1] != "write" {
		t.Fatalf("coverage syscalls = %v", got)
	}
	snapshot, err := observation.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	rule := testResolvedRule(t)
	requirement := NewCoverageRequirement(in.Subject, in.Backend, observation.SourceRef(), in.Scope, in.Context)
	set, err := NewResolvedMandatoryRequirementSet(rule, in.Attempt, []MandatoryRequirement{requirement})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := EvaluateEligibility(EligibilityEvaluation{Rule: rule, Requirements: set, Snapshot: snapshot, EvaluationAt: in.ObservedAt})
	if err != nil || decision.Result() != EligibilityEligible {
		t.Fatalf("real path decision = %#v, err=%v", decision, err)
	}

	wrong := NewCoverageRequirement(in.Subject, in.Backend, "spo-seccompprofile:substituted:uid:42", in.Scope, in.Context)
	wrongSet, _ := NewResolvedMandatoryRequirementSet(rule, in.Attempt, []MandatoryRequirement{wrong})
	decision, err = EvaluateEligibility(EligibilityEvaluation{Rule: rule, Requirements: wrongSet, Snapshot: snapshot, EvaluationAt: in.ObservedAt})
	if err != nil || decision.Result() != EligibilityUnknown {
		t.Fatalf("substituted provenance = %#v, err=%v", decision, err)
	}
}

func TestSPOCoverageAuthorityFailsClosed(t *testing.T) {
	for name, coverage := range map[string]string{
		"unknown-field":   `{"version":"v1","total":2,"syscalls":{"read":2,"write":1},"complete":true}`,
		"missing-syscall": `{"version":"v1","total":2,"syscalls":{"read":2}}`,
		"unsupported":     `{"version":"v2","total":2,"syscalls":{"read":2,"write":1}}`,
	} {
		t.Run(name, func(t *testing.T) {
			attempt, scope, contextIdentity, _, _, _ := testInputs(t)
			now := time.Unix(1_800_000_000, 0).UTC()
			in := SPOCoverageObservationRequest{Source: spoimport.Source{Mode: spoimport.ModeMergedProvenance, RecordingName: "recording", RecordingNamespace: "recording-ns", ProfileName: "recording-tools"}, Target: spoimport.Target{Namespace: "target-ns", Pod: "target", Container: "tools"}, Attempt: attempt, Subject: "subject", Backend: "SECCOMP", Scope: scope, Context: contextIdentity, ObservedAt: now, ValidUntil: now.Add(time.Hour), SPOVersion: "pinned", SPONamespace: "spo-system", SPOImage: "spo@sha256:pinned"}
			client := fake.NewSimpleDynamicClient(runtime.NewScheme(), goldenSPOProfile(coverage), goldenSPODeployment())
			if _, err := ObserveSPOCoverage(context.Background(), client, in); err == nil {
				t.Fatal("non-authoritative coverage established a fact")
			}
		})
	}
	if _, err := (SPOCoverageObservation{}).Snapshot(); err == nil {
		t.Fatal("detached/fabricated observation produced a snapshot")
	}
}
