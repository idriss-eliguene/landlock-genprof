package association

import (
	"reflect"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/history"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

func target(namespace, group, kind, name, container string) k8s.GovernedTarget {
	return k8s.GovernedTarget{Namespace: namespace, Workload: k8s.WorkloadRef{Group: group, Kind: kind, Name: name}, Container: container}
}

func completeEvidence(t k8s.GovernedTarget, image, binary string) Evidence {
	return Evidence{Target: &t, Population: history.Population{Qualified: true, Target: t.LegacyString(), Container: t.Container, ImageIdentity: image, BinaryPath: binary, RunsRecorded: 3}}
}

func completeProposal(t k8s.GovernedTarget) Proposal {
	return Proposal{Namespace: t.Namespace, Name: "proposal", Target: &t, Spec: proposal.Spec{Container: t.Container, Binary: "/app/server"}, Status: &proposal.Status{ApprovalState: proposal.ApprovalApproved}}
}

func TestAssociationSeparatesCanonicalTargetFields(t *testing.T) {
	base := target("team-a", "apps", "Deployment", "api", "app")
	tests := []struct {
		name   string
		mutate func(*k8s.GovernedTarget)
	}{
		{"namespace", func(v *k8s.GovernedTarget) { v.Namespace = "team-b" }},
		{"group", func(v *k8s.GovernedTarget) { v.Workload.Group = "example.io" }},
		{"kind", func(v *k8s.GovernedTarget) { v.Workload.Kind = "StatefulSet" }},
		{"name", func(v *k8s.GovernedTarget) { v.Workload.Name = "worker" }},
		{"container", func(v *k8s.GovernedTarget) { v.Container = "proxy" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			other := base
			test.mutate(&other)
			got := AssociateEvidence(base, completeEvidence(other, "sha256:x", "/app/server"))
			if got.State != Unassociated {
				t.Fatalf("state = %s, want %s", got.State, Unassociated)
			}
		})
	}
}

func TestLegacyEvidenceAndProposalFailClosed(t *testing.T) {
	target := target("team-a", "apps", "Deployment", "api", "app")
	legacyEvidence := Evidence{Population: history.Population{Target: "Deployment/api", Container: "app", ImageIdentity: "sha256:x", BinaryPath: "/app/server"}}
	if got := AssociateEvidence(target, legacyEvidence); got.State != InsufficientProvenance {
		t.Fatalf("legacy evidence = %+v", got)
	}
	legacyProposal := Proposal{Namespace: "team-a", Name: "api-app", Spec: proposal.Spec{Container: "app", Binary: "/app/server"}, Status: &proposal.Status{ApprovalState: proposal.ApprovalApproved}}
	if got := AssociateProposal(target, legacyProposal); got.State != InsufficientProvenance {
		t.Fatalf("legacy proposal = %+v", got)
	}
	// Current-cluster uniqueness, metadata names, and approval state cannot repair
	// missing historical identity.
	if got := AssociateProposal(target, Proposal{Namespace: target.Namespace, Name: target.Workload.Name, Status: &proposal.Status{ApprovalState: proposal.ApprovalRejected}}); got.State != InsufficientProvenance {
		t.Fatalf("name heuristic = %+v", got)
	}
}

func TestExplicitAssociationsIgnorePopulationAndGovernanceDimensions(t *testing.T) {
	target := target("team-a", "apps", "Deployment", "api", "app")
	evidence := completeEvidence(target, "sha256:a", "/app/server")
	evidence.Population.RunsRecorded = 1
	evidence.Population.Qualified = false
	if got := AssociateEvidence(target, evidence); got.State != Associated {
		t.Fatalf("evidence association = %+v", got)
	}
	for _, state := range []proposal.ApprovalState{proposal.ApprovalDraft, proposal.ApprovalReviewed, proposal.ApprovalApproved, proposal.ApprovalRejected} {
		source := completeProposal(target)
		source.Status.ApprovalState = state
		if got := AssociateProposal(target, source); got.State != Associated {
			t.Fatalf("approval %s changed association: %+v", state, got)
		}
	}
}

func TestAssociationToTargetsDistinguishesOrphanedAndOrder(t *testing.T) {
	baseTarget := target("team-a", "apps", "Deployment", "api", "app")
	source := completeEvidence(baseTarget, "sha256:a", "/app/server")
	if got := AssociateEvidenceToTargets(source, nil); got.State != Orphaned {
		t.Fatalf("orphan = %+v", got)
	}
	first := target("team-a", "apps", "Deployment", "other", "app")
	ordered := []k8s.GovernedTarget{first, baseTarget}
	reversed := []k8s.GovernedTarget{baseTarget, first}
	if left, right := AssociateEvidenceToTargets(source, ordered), AssociateEvidenceToTargets(source, reversed); !reflect.DeepEqual(left, right) || left.State != Associated {
		t.Fatalf("order changed result: %v / %v", left, right)
	}
	malformed := source
	malformed.Target = nil
	if got := AssociateEvidenceToTargets(malformed, []k8s.GovernedTarget{baseTarget}); got.State != InsufficientProvenance {
		t.Fatalf("malformed source = %+v", got)
	}
}

func TestRuntimePopulationCompatibilityIsSeparate(t *testing.T) {
	target := target("team-a", "apps", "Deployment", "api", "app")
	source := completeEvidence(target, "sha256:a", "/app/server")
	if got := CompareRuntimePopulation(source, k8s.RuntimeSubject{Target: target, ImageID: "sha256:b", BinaryPath: "/app/server"}); got.State != RuntimeDiffers {
		t.Fatalf("different image = %+v", got)
	}
	if got := CompareRuntimePopulation(source, k8s.RuntimeSubject{Target: target, ImageID: "", BinaryPath: "/app/server"}); got.State != RuntimeUnknown {
		t.Fatalf("unknown image = %+v", got)
	}
	if got := CompareRuntimePopulation(source, k8s.RuntimeSubject{Target: target, ImageID: "sha256:a", BinaryPath: "/app/worker"}); got.State != RuntimeDiffers {
		t.Fatalf("different binary = %+v", got)
	}
	if got := CompareRuntimePopulation(source, k8s.RuntimeSubject{Target: target, ImageID: "sha256:a", BinaryPath: "/app/server"}); got.State != RuntimeMatches {
		t.Fatalf("matching population = %+v", got)
	}
	if got := CompareRuntimePopulation(completeEvidence(target, "", "/app/server"), k8s.RuntimeSubject{Target: target, ImageID: "", BinaryPath: "/app/server"}); got.State != RuntimeUnknown {
		t.Fatalf("empty/empty image = %+v", got)
	}
}

func TestTargetValidationRequiresCanonicalIdentity(t *testing.T) {
	base := target("team-a", "apps", "Deployment", "api", "app")
	fields := []func(*k8s.GovernedTarget){
		func(v *k8s.GovernedTarget) { v.Namespace = "" },
		func(v *k8s.GovernedTarget) { v.Workload.Kind = "" },
		func(v *k8s.GovernedTarget) { v.Workload.Name = "" },
		func(v *k8s.GovernedTarget) { v.Container = "" },
	}
	for _, mutate := range fields {
		invalid := base
		mutate(&invalid)
		if got := AssociateEvidence(base, completeEvidence(invalid, "sha256:x", "/app/server")); got.State != InsufficientProvenance {
			t.Fatalf("invalid target = %+v", got)
		}
	}
}

func TestEvidenceTargetAssociationDoesNotDependOnImageOrPodUID(t *testing.T) {
	target := target("team-a", "apps", "Deployment", "api", "app")
	a := completeEvidence(target, "sha256:a", "/app/server")
	b := completeEvidence(target, "sha256:b", "/app/server")
	a.Population.Contributors = []string{"pod-a"}
	b.Population.Contributors = []string{"pod-b"}
	if AssociateEvidence(target, a).State != Associated || AssociateEvidence(target, b).State != Associated {
		t.Fatal("image or PodUID provenance changed logical target association")
	}
	if got := CompareRuntimePopulation(b, k8s.RuntimeSubject{Target: target, PodUID: "pod-b", ImageID: "sha256:b", BinaryPath: "/app/server"}); got.State != RuntimeMatches {
		t.Fatalf("current population = %+v", got)
	}
}
