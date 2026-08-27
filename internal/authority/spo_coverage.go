package authority

import (
	"context"
	"fmt"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/idriss-eliguene/landlock-genprof/internal/spoimport"
)

// SPOCoverageObservation is an opaque authority result produced only by the
// real SPO import boundary. Its fields cannot be populated by callers.
type SPOCoverageObservation struct {
	fact      CoverageFact
	sourceRef string
	profile   string
	coverage  spoimport.Coverage
}

type SPOCoverageObservationRequest struct {
	Source       spoimport.Source
	Target       spoimport.Target
	Attempt      ResolutionAttemptIdentity
	Subject      string
	Backend      string
	Scope        Scope
	Context      SecurityContextIdentity
	ObservedAt   time.Time
	ValidUntil   time.Time
	SPOVersion   string
	SPONamespace string
	SPOImage     string
}

// ObserveSPOCoverage reads and validates the named cluster objects itself.
// There is deliberately no constructor accepting annotation bytes or a
// caller-created positive coverage state.
func ObserveSPOCoverage(ctx context.Context, dyn dynamic.Interface, in SPOCoverageObservationRequest) (SPOCoverageObservation, error) {
	if dyn == nil || !in.Attempt.Valid() || in.Subject == "" || in.Backend == "" || !in.Scope.Valid() || !validContext(in.Context) || in.ObservedAt.IsZero() || in.ValidUntil.IsZero() || in.ValidUntil.Before(in.ObservedAt) || in.SPOVersion == "" || in.SPONamespace == "" || in.SPOImage == "" {
		return SPOCoverageObservation{}, fmt.Errorf("invalid SPO coverage observation request")
	}
	deployment, err := dyn.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}).Namespace(in.SPONamespace).Get(ctx, "security-profiles-operator", metav1.GetOptions{})
	if err != nil {
		return SPOCoverageObservation{}, fmt.Errorf("reading SPO producer deployment: %w", err)
	}
	containers, found, err := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", "containers")
	if err != nil || !found {
		return SPOCoverageObservation{}, fmt.Errorf("SPO producer image unavailable")
	}
	imageBound := false
	for _, item := range containers {
		container, ok := item.(map[string]interface{})
		if ok && container["name"] == "security-profiles-operator" && container["image"] == in.SPOImage {
			imageBound = true
		}
	}
	if !imageBound {
		return SPOCoverageObservation{}, fmt.Errorf("SPO producer image binding mismatch")
	}
	imported, err := spoimport.Import(ctx, dyn, in.Source, in.Target)
	if err != nil {
		return SPOCoverageObservation{}, err
	}
	if imported.Coverage.State != spoimport.CoverageKnown || imported.SourceProfileUID == "" || imported.SourceProfileResourceVersion == "" {
		return SPOCoverageObservation{}, fmt.Errorf("SPO coverage authority unavailable")
	}
	profileSyscalls := map[string]struct{}{}
	for _, rule := range imported.Profile.Syscalls {
		for _, name := range rule.Names {
			profileSyscalls[name] = struct{}{}
		}
	}
	if len(profileSyscalls) == 0 || len(profileSyscalls) != len(imported.Coverage.Syscalls) {
		return SPOCoverageObservation{}, fmt.Errorf("SPO coverage does not bind the generated profile syscall set")
	}
	for name := range profileSyscalls {
		if imported.Coverage.Syscalls[name] <= 0 {
			return SPOCoverageObservation{}, fmt.Errorf("SPO coverage omits generated syscall %q", name)
		}
	}

	validity, _ := NewValidity(in.ObservedAt, &in.ValidUntil, 0)
	digest, _ := NewDigest("sha256:9af7d3897e349b51f4554d1536c3984e71489bc823d7c93e52a3eddf81f59fb0")
	verifier, _ := NewVerifierSemanticIdentity(VerifierSemanticIdentity{
		id: "security-profiles-operator", version: in.SPOVersion, digest: digest,
		class: "SPO_SECCOMP_RECORDER", inputSchema: "SeccompProfile.v1", outputSchema: "syscall-coverage.v1",
		property: "SCOPE_COVERAGE", procedure: "PROFILE_RECORDING_MERGE",
	})
	sourceRef := fmt.Sprintf("spo-seccompprofile:%s:%s:%s", in.Source.ProfileName, imported.SourceProfileUID, imported.SourceProfileResourceVersion)
	provenance, err := NewProvenanceRecord("security-profiles-operator", "ProfileRecording/Bpf/Containers", in.SPOVersion, sourceRef, in.Scope, validity, RevocationNotRevoked, verifier)
	if err != nil {
		return SPOCoverageObservation{}, err
	}
	revResult, err := newCurrentRevocationResult(in.Attempt, in.Subject, sourceRef, RevocationNotRevoked, provenance, validity)
	if err != nil {
		return SPOCoverageObservation{}, err
	}
	revocation, err := DeriveCurrentRevocationFact(revResult)
	if err != nil {
		return SPOCoverageObservation{}, err
	}
	result, err := newCoverageObservationResult(in.Attempt, in.Subject, in.Backend, sourceRef, in.Scope, in.Context, validity, revocation, ScopeCovers, provenance)
	if err != nil {
		return SPOCoverageObservation{}, err
	}
	fact, err := DeriveCoverageFact(result)
	if err != nil {
		return SPOCoverageObservation{}, err
	}
	return SPOCoverageObservation{fact: fact, sourceRef: sourceRef, profile: in.Source.ProfileName, coverage: imported.Coverage}, nil
}

func (o SPOCoverageObservation) SourceRef() string { return o.sourceRef }
func (o SPOCoverageObservation) Profile() string   { return o.profile }
func (o SPOCoverageObservation) CoverageSyscalls() []string {
	out := make([]string, 0, len(o.coverage.Syscalls))
	for name := range o.coverage.Syscalls {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
func (o SPOCoverageObservation) Snapshot() (EvaluationFactSnapshot, error) {
	if !o.fact.Valid() || o.sourceRef == "" {
		return EvaluationFactSnapshot{}, fmt.Errorf("invalid SPO coverage observation")
	}
	return NewEvaluationFactSnapshotAll(o.fact.attempt, nil, nil, nil, nil, []CoverageFact{o.fact}, nil, nil, nil)
}
