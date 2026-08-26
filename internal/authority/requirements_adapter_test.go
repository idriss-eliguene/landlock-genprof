package authority

import (
	"reflect"
	"testing"
)

func TestAuthoritativeRequirementAdapterEightFamilies(t *testing.T) {
	bound := testResolvedRule(t)
	attempt, scope, context, validity, provenance, revocation := testInputs(t)
	members, verifier := testRequirementMembers(t)
	set, err := NewResolvedMandatoryRequirementSet(bound, attempt, members)
	if err != nil {
		t.Fatal(err)
	}
	authorityID, _ := canonicalIdentityString(referenceJSON(bound.Reference()))

	trust := TrustFact{attempt: attempt, subject: "subject", policy: "policy", root: "root", scope: scope, context: context, validity: validity, revocation: revocation, provenance: provenance, state: Trusted}
	verification := ResolvedVerificationFact{attempt: attempt, subject: "subject", verifier: "verifier", property: "property", scope: scope, context: context, validity: validity, revocation: revocation, provenance: provenance, state: VerificationFactVerified}
	revocationFact := CurrentRevocationFact{attempt: attempt, subject: "subject", source: "source", state: RevocationNotRevoked, provenance: provenance, validUntil: validity}
	coverage := CoverageFact{attempt: attempt, subject: "subject", backend: "SECCOMP", source: "source", scope: scope, context: context, validity: validity, revocation: revocation, state: ScopeCovers, provenance: provenance}
	completeness := CompletenessFact{attempt: attempt, subject: "subject", class: EmpiricalCompleteness, scope: scope, provenance: provenance, validity: validity, revocation: revocation}
	adequacy := AdequacyFact{attempt: attempt, subject: "subject", class: StructuralBaseline, scope: scope, context: context, verifier: verifier, provenance: provenance, validity: validity, revocation: revocation}
	certification := CertificationFact{attempt: attempt, subject: "subject", identity: "certificate", property: "1", scope: scope, context: context, verifier: verifier, provenance: provenance, validity: validity, revocation: revocation}

	for _, member := range members {
		t.Run(string(member.Family), func(t *testing.T) {
			request, err := set.MatchRequest(member, validity.ObservedAt())
			if err != nil {
				t.Fatal(err)
			}
			memberID, _ := member.MemberIdentity()
			if request.Authority != authorityID || request.Requirement != string(memberID) || request.Attempt != attempt || !request.At.Equal(validity.ObservedAt()) || request.Subject != "subject" {
				t.Fatalf("base binding lost: %#v", request)
			}
			var snapshot EvaluationFactSnapshot
			snapshot.attempt = attempt
			switch member.Family {
			case RequirementTrust:
				if request.Policy != "policy" || request.Root != "root" || request.TypedContext != context {
					t.Fatal("trust operands lost")
				}
				snapshot.trusts = []TrustFact{trust}
			case RequirementVerification:
				if request.Producer != "verifier" || request.Property != "property" || request.TypedContext != context {
					t.Fatal("verification operands lost")
				}
				snapshot.verifications = []ResolvedVerificationFact{verification}
			case RequirementRevocation:
				if request.Source != "source" {
					t.Fatal("revocation source lost")
				}
				snapshot.revocations = []CurrentRevocationFact{revocationFact}
			case RequirementCompatibility:
				if request.CompatibilityRequirementRef != "compat-ref" || request.Backend != "SECCOMP" || request.Context == "" || request.TypedContext != context {
					t.Fatal("compatibility operands lost")
				}
				snapshot.compatibilities = []CompatibilityFact{{attempt: attempt, schema: request.Schema, predicate: request.Predicate, field: request.Field, candidate: request.Candidate, baseline: request.Baseline, requirement: request.CompatibilityRequirementRef, authority: request.Authority, subject: request.Subject, backend: request.Backend, context: request.Context, scope: request.Scope, validity: validity, revocation: revocation, provenance: provenance, state: CompatibilityCompatible}}
			case RequirementCoverage:
				if request.Backend != "SECCOMP" || request.Source != "source" || request.TypedContext != context {
					t.Fatal("coverage operands lost")
				}
				snapshot.coverages = []CoverageFact{coverage}
			case RequirementCompleteness:
				if request.RequiredCompletenessClass != EmpiricalCompleteness {
					t.Fatal("completeness class lost")
				}
				snapshot.completeness = []CompletenessFact{completeness}
			case RequirementAdequacy:
				if request.RequiredAdequacyClass != StructuralBaseline || request.TypedContext != context || !reflect.DeepEqual(request.Verifier, verifier) {
					t.Fatal("adequacy operands lost")
				}
				snapshot.adequacies = []AdequacyFact{adequacy}
			case RequirementCertification:
				if request.Source != "certificate" || request.Property != "1" || request.TypedContext != context || !reflect.DeepEqual(request.Verifier, verifier) {
					t.Fatal("certification operands lost")
				}
				snapshot.certifications = []CertificationFact{certification}
			}
			match, err := MatchSnapshot(request, snapshot)
			if err != nil || match.outcome != MatchSatisfied {
				t.Fatalf("adapter positive control did not satisfy: request=%#v match=%#v %v", request, match, err)
			}
		})
	}

	detached := NewRevocationStatusRequirement("subject", "other-source")
	if _, err := set.MatchRequest(detached, validity.ObservedAt()); err == nil {
		t.Fatal("detached requirement acquired authority")
	}
}

func TestContextAndVerifierSubstitutionsDoNotSatisfy(t *testing.T) {
	attempt, scope, context, validity, provenance, revocation := testInputs(t)
	_, verifier := testRequirementMembers(t)
	otherContext, _ := NewSecurityContextIdentity(SecurityContextIdentity{ImageIdentity: "other", Architecture: "arch", ABI: "abi", KernelRuntimeClass: "kernel", WorkloadIdentity: "workload", ExecutableIdentity: "exe"})
	otherVerifier := verifier
	otherVerifier.id = "other-verifier"

	cases := []struct {
		name     string
		request  MatchRequest
		snapshot EvaluationFactSnapshot
		mutate   func(*MatchRequest)
	}{
		{"trust-context", MatchRequest{Family: FamilyTrust, Attempt: attempt, Authority: "a", Requirement: "r", Subject: "subject", Policy: "policy", Root: "root", Scope: scope, TypedContext: context, At: validity.ObservedAt()}, EvaluationFactSnapshot{attempt: attempt, trusts: []TrustFact{{attempt: attempt, subject: "subject", policy: "policy", root: "root", scope: scope, context: context, validity: validity, revocation: revocation, provenance: provenance, state: Trusted}}}, func(r *MatchRequest) { r.TypedContext = otherContext }},
		{"verification-context", MatchRequest{Family: FamilyVerification, Attempt: attempt, Authority: "a", Requirement: "r", Subject: "subject", Producer: "verifier", Property: "property", Scope: scope, TypedContext: context, At: validity.ObservedAt()}, EvaluationFactSnapshot{attempt: attempt, verifications: []ResolvedVerificationFact{{attempt: attempt, subject: "subject", verifier: "verifier", property: "property", scope: scope, context: context, validity: validity, revocation: revocation, provenance: provenance, state: VerificationFactVerified}}}, func(r *MatchRequest) { r.TypedContext = otherContext }},
		{"adequacy-context", MatchRequest{Family: FamilyAdequacy, Attempt: attempt, Authority: "a", Requirement: "r", Subject: "subject", Scope: scope, TypedContext: context, Verifier: verifier, RequiredAdequacyClass: StructuralBaseline, At: validity.ObservedAt()}, EvaluationFactSnapshot{attempt: attempt, adequacies: []AdequacyFact{{attempt: attempt, subject: "subject", class: StructuralBaseline, scope: scope, context: context, verifier: verifier, provenance: provenance, validity: validity, revocation: revocation}}}, func(r *MatchRequest) { r.TypedContext = otherContext }},
		{"adequacy-verifier", MatchRequest{Family: FamilyAdequacy, Attempt: attempt, Authority: "a", Requirement: "r", Subject: "subject", Scope: scope, TypedContext: context, Verifier: verifier, RequiredAdequacyClass: StructuralBaseline, At: validity.ObservedAt()}, EvaluationFactSnapshot{attempt: attempt, adequacies: []AdequacyFact{{attempt: attempt, subject: "subject", class: StructuralBaseline, scope: scope, context: context, verifier: verifier, provenance: provenance, validity: validity, revocation: revocation}}}, func(r *MatchRequest) { r.Verifier = otherVerifier }},
		{"certification-context", MatchRequest{Family: FamilyCertification, Attempt: attempt, Authority: "a", Requirement: "r", Subject: "subject", Source: "certificate", Property: "property", Scope: scope, TypedContext: context, Verifier: verifier, At: validity.ObservedAt()}, EvaluationFactSnapshot{attempt: attempt, certifications: []CertificationFact{{attempt: attempt, subject: "subject", identity: "certificate", property: "property", scope: scope, context: context, verifier: verifier, provenance: provenance, validity: validity, revocation: revocation}}}, func(r *MatchRequest) { r.TypedContext = otherContext }},
		{"certification-verifier", MatchRequest{Family: FamilyCertification, Attempt: attempt, Authority: "a", Requirement: "r", Subject: "subject", Source: "certificate", Property: "property", Scope: scope, TypedContext: context, Verifier: verifier, At: validity.ObservedAt()}, EvaluationFactSnapshot{attempt: attempt, certifications: []CertificationFact{{attempt: attempt, subject: "subject", identity: "certificate", property: "property", scope: scope, context: context, verifier: verifier, provenance: provenance, validity: validity, revocation: revocation}}}, func(r *MatchRequest) { r.Verifier = otherVerifier }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			positive, err := MatchSnapshot(tc.request, tc.snapshot)
			if err != nil || positive.outcome != MatchSatisfied {
				t.Fatalf("positive control: %#v %v", positive, err)
			}
			tc.mutate(&tc.request)
			negative, err := MatchSnapshot(tc.request, tc.snapshot)
			if err == nil && negative.outcome == MatchSatisfied {
				t.Fatal("substitution satisfied")
			}
		})
	}
}
