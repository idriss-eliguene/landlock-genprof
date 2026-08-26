package authority

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/idriss-eliguene/landlock-genprof/internal/spoimport"
)

// TestGoldenRealSPOCoverageEligibility is opt-in because its evidence must
// come from a live recorder, never fixtures. The E2E script sets these values
// only after independently verifying the generated profile and annotation.
func TestGoldenRealSPOCoverageEligibility(t *testing.T) {
	if os.Getenv("SPO_GOLDEN_LIVE") != "1" {
		t.Skip("requires the real SPO Golden E2E")
	}
	requiredEnv := func(name string) string {
		t.Helper()
		value := os.Getenv(name)
		if value == "" {
			t.Fatalf("%s is required", name)
		}
		return value
	}
	config, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	_, scope, securityContext, _, _, _ := testInputs(t)
	attempt, err := NewResolutionAttemptIdentity(requiredEnv("SPO_GOLDEN_ATTEMPT"))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	subject := requiredEnv("SPO_GOLDEN_SUBJECT")
	observation, err := ObserveSPOCoverage(context.Background(), client, SPOCoverageObservationRequest{
		Source: spoimport.Source{
			Mode: spoimport.ModeMergedProvenance, RecordingName: requiredEnv("SPO_GOLDEN_RECORDING"),
			RecordingNamespace: requiredEnv("SPO_GOLDEN_NAMESPACE"), ProfileName: requiredEnv("SPO_GOLDEN_PROFILE"),
		},
		Target:  spoimport.Target{Namespace: requiredEnv("SPO_GOLDEN_NAMESPACE"), Pod: requiredEnv("SPO_GOLDEN_POD"), Container: requiredEnv("SPO_GOLDEN_CONTAINER")},
		Attempt: attempt, Subject: subject, Backend: "SECCOMP", Scope: scope, Context: securityContext,
		ObservedAt: at, ValidUntil: at.Add(10 * time.Minute), SPOVersion: requiredEnv("SPO_GOLDEN_VERSION"),
		SPONamespace: requiredEnv("SPO_GOLDEN_SPO_NAMESPACE"), SPOImage: requiredEnv("SPO_GOLDEN_IMAGE"),
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := observation.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	rule := testResolvedRule(t)
	requirement := NewCoverageRequirement(subject, "SECCOMP", observation.SourceRef(), scope, securityContext)
	set, err := NewResolvedMandatoryRequirementSet(rule, attempt, []MandatoryRequirement{requirement})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := EvaluateEligibility(EligibilityEvaluation{Rule: rule, Requirements: set, Snapshot: snapshot, EvaluationAt: at})
	if err != nil || decision.Result() != EligibilityEligible {
		t.Fatalf("P3 result=%v error=%v", decision.Result(), err)
	}

	forged := SPOCoverageObservation{}
	if _, err := forged.Snapshot(); err == nil {
		t.Fatal("caller-fabricated coverage minted an authoritative snapshot")
	}
	wrongRequirement := NewCoverageRequirement(subject, "SECCOMP", observation.SourceRef()+":substituted", scope, securityContext)
	wrongSet, _ := NewResolvedMandatoryRequirementSet(rule, attempt, []MandatoryRequirement{wrongRequirement})
	wrongDecision, err := EvaluateEligibility(EligibilityEvaluation{Rule: rule, Requirements: wrongSet, Snapshot: snapshot, EvaluationAt: at})
	if err != nil || wrongDecision.Result() == EligibilityEligible {
		t.Fatalf("provenance substitution result=%v error=%v", wrongDecision.Result(), err)
	}

	fmt.Printf("REAL_SPO profile=%s sourceRef=%s syscalls=%d match=SATISFIED p3=ELIGIBLE forged=REJECTED substituted=%v decision=%s\n",
		observation.Profile(), observation.SourceRef(), len(observation.CoverageSyscalls()), wrongDecision.Result(), decision.ID())
}
