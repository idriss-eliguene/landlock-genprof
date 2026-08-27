package authority

import (
	"encoding/json"
	"testing"
	"time"
)

type p3Fixture struct {
	rule     TypedResolvedAuthorityRule
	set      ResolvedMandatoryRequirementSet
	snapshot EvaluationFactSnapshot
	at       time.Time
	members  []MandatoryRequirement
}

func newP3Fixture(t *testing.T) p3Fixture {
	t.Helper()
	rule := testResolvedRule(t)
	attempt, scope, context, validity, provenance, revocation := testInputs(t)
	members, verifier := testRequirementMembers(t)
	set, err := NewResolvedMandatoryRequirementSet(rule, attempt, members)
	if err != nil {
		t.Fatal(err)
	}
	authorityID, _ := canonicalIdentityString(referenceJSON(rule.Reference()))
	contextID, _ := canonicalIdentityString(contextIdentity(context))
	snapshot, err := NewEvaluationFactSnapshotAll(attempt,
		[]TrustFact{{attempt: attempt, subject: "subject", policy: "policy", root: "root", scope: scope, context: context, validity: validity, revocation: revocation, provenance: provenance, state: Trusted}},
		[]CurrentRevocationFact{{attempt: attempt, subject: "subject", source: "source", state: RevocationNotRevoked, provenance: provenance, validUntil: validity}},
		[]ResolvedVerificationFact{{attempt: attempt, subject: "subject", verifier: "verifier", property: "property", scope: scope, context: context, validity: validity, revocation: revocation, provenance: provenance, state: VerificationFactVerified}},
		[]CompatibilityFact{{attempt: attempt, schema: string(CompatibilityRuleV2), predicate: "SET_CONTAINS", field: "field", candidate: "candidate", baseline: "baseline", requirement: "compat-ref", authority: authorityID, subject: "subject", backend: "SECCOMP", context: contextID, scope: scope, validity: validity, revocation: revocation, provenance: provenance, state: CompatibilityCompatible}},
		[]CoverageFact{{attempt: attempt, subject: "subject", backend: "SECCOMP", source: "source", scope: scope, context: context, validity: validity, revocation: revocation, state: ScopeCovers, provenance: provenance}},
		[]CompletenessFact{{attempt: attempt, subject: "subject", class: EmpiricalCompleteness, scope: scope, provenance: provenance, validity: validity, revocation: revocation}},
		[]AdequacyFact{{attempt: attempt, subject: "subject", class: StructuralBaseline, scope: scope, context: context, verifier: verifier, provenance: provenance, validity: validity, revocation: revocation}},
		[]CertificationFact{{attempt: attempt, subject: "subject", identity: "certificate", property: "1", scope: scope, context: context, verifier: verifier, provenance: provenance, validity: validity, revocation: revocation}},
	)
	if err != nil {
		t.Fatal(err)
	}
	return p3Fixture{rule: rule, set: set, snapshot: snapshot, at: validity.ObservedAt(), members: members}
}

func evaluateP3(t *testing.T, fixture p3Fixture) EligibilityDecision {
	t.Helper()
	decision, err := EvaluateEligibility(EligibilityEvaluation{Rule: fixture.rule, Requirements: fixture.set, Snapshot: fixture.snapshot, EvaluationAt: fixture.at})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Valid() {
		t.Fatal("invalid eligibility decision")
	}
	return decision
}

func TestEligibilityEightFamilyAllSatisfied(t *testing.T) {
	f := newP3Fixture(t)
	decision := evaluateP3(t, f)
	if decision.Result() != EligibilityEligible || decision.RuleRef() != f.rule.Reference() || decision.RequirementSetID() != f.set.ID() || decision.Attempt() != f.set.Attempt() || !decision.EvaluationAt().Equal(f.at) {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestEligibilityAggregation(t *testing.T) {
	t.Run("refuted", func(t *testing.T) {
		f := newP3Fixture(t)
		f.snapshot.trusts[0].state = Untrusted
		if got := evaluateP3(t, f).Result(); got != EligibilityIneligible {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("unknown", func(t *testing.T) {
		f := newP3Fixture(t)
		f.snapshot.trusts[0].state = TrustUnknown
		if got := evaluateP3(t, f).Result(); got != EligibilityUnknown {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("non-matching", func(t *testing.T) {
		f := newP3Fixture(t)
		f.snapshot.trusts[0].subject = "other"
		if got := evaluateP3(t, f).Result(); got != EligibilityUnknown {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("missing", func(t *testing.T) {
		f := newP3Fixture(t)
		f.snapshot.trusts = nil
		if got := evaluateP3(t, f).Result(); got != EligibilityUnknown {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("refuted-dominates-unknown", func(t *testing.T) {
		f := newP3Fixture(t)
		f.snapshot.trusts[0].state = Untrusted
		f.snapshot.verifications = nil
		if got := evaluateP3(t, f).Result(); got != EligibilityIneligible {
			t.Fatalf("got %v", got)
		}
	})
}

func TestEligibilityExtraFactsDoNotAddRequirements(t *testing.T) {
	f := newP3Fixture(t)
	extra := f.snapshot.trusts[0]
	extra.subject = "irrelevant"
	f.snapshot.trusts = append(f.snapshot.trusts, extra)
	if got := evaluateP3(t, f).Result(); got != EligibilityEligible {
		t.Fatalf("got %v", got)
	}
	if len(f.set.Requirements()) != 8 {
		t.Fatal("required membership changed")
	}
}

func TestEligibilityRejectsIncoherentInputs(t *testing.T) {
	f := newP3Fixture(t)
	t.Run("zero-time", func(t *testing.T) {
		_, err := EvaluateEligibility(EligibilityEvaluation{Rule: f.rule, Requirements: f.set, Snapshot: f.snapshot})
		if err == nil {
			t.Fatal("zero EvaluationAt accepted")
		}
	})
	t.Run("wrong-attempt", func(t *testing.T) {
		other, _ := NewResolutionAttemptIdentity("other")
		snapshot := f.snapshot
		snapshot.attempt = other
		_, err := EvaluateEligibility(EligibilityEvaluation{Rule: f.rule, Requirements: f.set, Snapshot: snapshot, EvaluationAt: f.at})
		if err == nil {
			t.Fatal("wrong attempt accepted")
		}
	})
	t.Run("wrong-rule", func(t *testing.T) {
		other := f.rule
		other.reference.id = "other-rule"
		_, err := EvaluateEligibility(EligibilityEvaluation{Rule: other, Requirements: f.set, Snapshot: f.snapshot, EvaluationAt: f.at})
		if err == nil {
			t.Fatal("wrong rule accepted")
		}
	})
	t.Run("changed-version-digest", func(t *testing.T) {
		other := f.rule
		other.reference.version.parts[3]++
		other.reference.digest = Digest("sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
		_, err := EvaluateEligibility(EligibilityEvaluation{Rule: other, Requirements: f.set, Snapshot: f.snapshot, EvaluationAt: f.at})
		if err == nil {
			t.Fatal("changed rule version/digest accepted")
		}
	})
	t.Run("invalid-snapshot", func(t *testing.T) {
		_, err := EvaluateEligibility(EligibilityEvaluation{Rule: f.rule, Requirements: f.set, Snapshot: EvaluationFactSnapshot{}, EvaluationAt: f.at})
		if err == nil {
			t.Fatal("invalid snapshot accepted")
		}
	})
	t.Run("changed-set-id", func(t *testing.T) {
		changed := f.set
		changed.id = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
		_, err := EvaluateEligibility(EligibilityEvaluation{Rule: f.rule, Requirements: changed, Snapshot: f.snapshot, EvaluationAt: f.at})
		if err == nil {
			t.Fatal("changed requirement-set ID accepted")
		}
	})
}

func TestEligibilityEightFamilyNegativeMatrix(t *testing.T) {
	cases := []struct {
		name   string
		want   EligibilityResult
		mutate func(*EvaluationFactSnapshot)
	}{
		{"trust", EligibilityIneligible, func(s *EvaluationFactSnapshot) { s.trusts[0].state = Untrusted }},
		{"verification", EligibilityIneligible, func(s *EvaluationFactSnapshot) { s.verifications[0].state = VerificationFactFailed }},
		{"revocation", EligibilityIneligible, func(s *EvaluationFactSnapshot) { s.revocations[0].state = RevocationRevoked }},
		{"compatibility", EligibilityIneligible, func(s *EvaluationFactSnapshot) { s.compatibilities[0].state = CompatibilityIncompatible }},
		{"coverage", EligibilityIneligible, func(s *EvaluationFactSnapshot) { s.coverages[0].state = ScopeDoesNotCover }},
		{"completeness", EligibilityUnknown, func(s *EvaluationFactSnapshot) { s.completeness[0].class = StructuralCompleteness }},
		{"adequacy", EligibilityUnknown, func(s *EvaluationFactSnapshot) { s.adequacies[0].class = ExternalCertification }},
		{"certification", EligibilityUnknown, func(s *EvaluationFactSnapshot) { s.certifications = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newP3Fixture(t)
			tc.mutate(&f.snapshot)
			if got := evaluateP3(t, f).Result(); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestEligibilityCanonicalMembershipAndReplayIdentity(t *testing.T) {
	f := newP3Fixture(t)
	base := evaluateP3(t, f)
	reordered := append([]MandatoryRequirement(nil), f.members...)
	for i, j := 0, len(reordered)-1; i < j; i, j = i+1, j-1 {
		reordered[i], reordered[j] = reordered[j], reordered[i]
	}
	reordered = append(reordered, reordered[0], reordered[0])
	set2, err := NewResolvedMandatoryRequirementSet(f.rule, f.set.Attempt(), reordered)
	if err != nil {
		t.Fatal(err)
	}
	f.set = set2
	canonical := evaluateP3(t, f)
	if canonical.ID() != base.ID() || canonical.RequirementSetID() != base.RequirementSetID() {
		t.Fatal("permutation or duplicates changed decision identity")
	}

	timeReplay := f
	timeReplay.at = f.at.Add(time.Nanosecond)
	if evaluateP3(t, timeReplay).ID() == base.ID() {
		t.Fatal("changed EvaluationAt preserved decision identity")
	}
	resultReplay := f
	resultReplay.snapshot.trusts[0].state = Untrusted
	if evaluateP3(t, resultReplay).ID() == base.ID() {
		t.Fatal("changed result preserved decision identity")
	}
	setReplay := f
	extra := NewRevocationStatusRequirement("subject", "second-source")
	setReplay.members = append(setReplay.members, extra)
	setReplay.set, err = NewResolvedMandatoryRequirementSet(f.rule, f.set.Attempt(), setReplay.members)
	if err != nil {
		t.Fatal(err)
	}
	if evaluateP3(t, setReplay).ID() == base.ID() {
		t.Fatal("changed requirement membership preserved decision identity")
	}

	attemptReplay := f
	otherAttempt, _ := NewResolutionAttemptIdentity("other-attempt")
	attemptReplay.set, err = NewResolvedMandatoryRequirementSet(f.rule, otherAttempt, f.members)
	if err != nil {
		t.Fatal(err)
	}
	attemptReplay.snapshot.attempt = otherAttempt
	for i := range attemptReplay.snapshot.trusts {
		attemptReplay.snapshot.trusts[i].attempt = otherAttempt
	}
	for i := range attemptReplay.snapshot.revocations {
		attemptReplay.snapshot.revocations[i].attempt = otherAttempt
	}
	for i := range attemptReplay.snapshot.verifications {
		attemptReplay.snapshot.verifications[i].attempt = otherAttempt
	}
	for i := range attemptReplay.snapshot.compatibilities {
		attemptReplay.snapshot.compatibilities[i].attempt = otherAttempt
	}
	for i := range attemptReplay.snapshot.coverages {
		attemptReplay.snapshot.coverages[i].attempt = otherAttempt
	}
	for i := range attemptReplay.snapshot.completeness {
		attemptReplay.snapshot.completeness[i].attempt = otherAttempt
	}
	for i := range attemptReplay.snapshot.adequacies {
		attemptReplay.snapshot.adequacies[i].attempt = otherAttempt
	}
	for i := range attemptReplay.snapshot.certifications {
		attemptReplay.snapshot.certifications[i].attempt = otherAttempt
	}
	if evaluateP3(t, attemptReplay).ID() == base.ID() {
		t.Fatal("changed attempt preserved decision identity")
	}

	ruleReplay := newP3Fixture(t)
	_, fields := decodeCompleteRule(t)
	fields["id"] = json.RawMessage(`"rule-2"`)
	fields["version"] = json.RawMessage(`"2.0.0.0"`)
	rule2, err := NewAuthorityRule(fields)
	if err != nil {
		t.Fatal(err)
	}
	digest2, err := AuthorityRuleDigestOf(rule2)
	if err != nil {
		t.Fatal(err)
	}
	ref2, err := NewSemanticReference(ObjectKindAuthorityRule, rule2.id, rule2.version, Digest(digest2))
	if err != nil {
		t.Fatal(err)
	}
	ruleReplay.rule = TypedResolvedAuthorityRule{reference: ref2, rule: rule2, digest: digest2}
	ruleReplay.set, err = NewResolvedMandatoryRequirementSet(ruleReplay.rule, ruleReplay.set.Attempt(), ruleReplay.members)
	if err != nil {
		t.Fatal(err)
	}
	ruleReplay.snapshot.compatibilities[0].authority, _ = canonicalIdentityString(referenceJSON(ref2))
	ruleDecision := evaluateP3(t, ruleReplay)
	if ruleDecision.Result() != EligibilityEligible || ruleDecision.ID() == base.ID() {
		t.Fatal("changed rule version/digest preserved decision identity")
	}
}

func TestEligibilityRejectsEmptySetAndDetachedAuthority(t *testing.T) {
	f := newP3Fixture(t)
	empty := f.set
	empty.requirements = nil
	if decision, err := EvaluateEligibility(EligibilityEvaluation{Rule: f.rule, Requirements: empty, Snapshot: f.snapshot, EvaluationAt: f.at}); err == nil || decision.Result() == EligibilityEligible {
		t.Fatal("empty set established eligibility")
	}
	detached := NewTrustRequirement("other", "policy", "root", f.members[0].Trust.Scope, f.members[0].Trust.Context)
	if _, err := f.set.MatchRequest(detached, f.at); err == nil {
		t.Fatal("detached requirement acquired authority")
	}
	// No P3 input field accepts ELIGIBLE, SATISFIED, complete=true, generic
	// predicate positives, or fabricated RequirementMatch values.
}
