package authority

import (
	"bytes"
	"math/rand"
	"testing"
	"time"
)

func TestResolutionStatesAndSplitBrain(t *testing.T) {
	d := testDigest(t)
	ref, _ := NewResolutionReference(KindAuthorityRule, "r", "v1", d)
	sa, _ := NewAuthoritySource("a", "memory", []ReferenceKind{KindAuthorityRule}, 1, d, true, SourceAuthoritative)
	sb, _ := NewAuthoritySource("b", "memory", []ReferenceKind{KindAuthorityRule}, 2, d, true, SourceAuthoritative)
	cfg, _ := NewSourceConfiguration([]AuthoritySource{sa, sb}, d)
	oa, _ := NewResolvedObject(ref, d, []byte("same"))
	ob, _ := NewResolvedObject(ref, d, []byte("other"))
	got := AggregateObservations(ref, cfg, []SourceObservation{{Source: sa, State: ResolutionResolved, Object: oa}, {Source: sb, State: ResolutionResolved, Object: oa}})
	if got.State() != ResolutionResolved || got.Object() == nil {
		t.Fatal("identical replicas not resolved")
	}
	got = AggregateObservations(ref, cfg, []SourceObservation{{Source: sa, State: ResolutionResolved, Object: oa}, {Source: sb, State: ResolutionResolved, Object: ob}})
	if got.State() != ResolutionAmbiguous {
		t.Fatalf("split brain = %v", got.State())
	}
	for i := 0; i < 100; i++ {
		obs := []SourceObservation{{Source: sa, State: ResolutionResolved, Object: oa}, {Source: sb, State: ResolutionResolved, Object: ob}}
		rand.New(rand.NewSource(int64(i))).Shuffle(len(obs), func(i, j int) { obs[i], obs[j] = obs[j], obs[i] })
		if AggregateObservations(ref, cfg, obs).State() != ResolutionAmbiguous {
			t.Fatal("order changed result")
		}
	}
}

func TestResolutionAbsenceUnavailableAndExactness(t *testing.T) {
	d := testDigest(t)
	ref, _ := NewResolutionReference(KindAuthorityRule, "r", "v1", d)
	s, _ := NewAuthoritySource("a", "memory", []ReferenceKind{KindAuthorityRule}, 1, d, true, SourceAuthoritative)
	cfg, _ := NewSourceConfiguration([]AuthoritySource{s}, d)
	if AggregateObservations(ref, cfg, []SourceObservation{{Source: s, State: ResolutionNotFound}}).State() != ResolutionNotFound {
		t.Fatal("not found mapping")
	}
	if AggregateObservations(ref, cfg, []SourceObservation{{Source: s, State: ResolutionUnavailable}}).State() != ResolutionUnavailable {
		t.Fatal("unavailable mapping")
	}
	if AggregateObservations(ref, cfg, []SourceObservation{{Source: s, State: ResolutionDigestMismatch}}).State() != ResolutionDigestMismatch {
		t.Fatal("single digest mismatch was not preserved")
	}
	wrongRef, _ := NewResolutionReference(KindBaseline, "r", "v1", d)
	wrongObject, _ := NewResolvedObject(wrongRef, d, []byte("wrong-kind"))
	if AggregateObservations(ref, cfg, []SourceObservation{{Source: s, State: ResolutionResolved, Object: wrongObject}}).State() != ResolutionTypeMismatch {
		t.Fatal("type mismatch was not preserved")
	}
	// An observation must use the exact configured source descriptor; a
	// same-named source with a different mode cannot influence resolution.
	if altered, err := NewAuthoritySource("a", "memory", []ReferenceKind{KindAuthorityRule}, 1, d, true, SourceMirror); err == nil {
		if AggregateObservations(ref, cfg, []SourceObservation{{Source: altered, State: ResolutionUnavailable}}).State() != ResolutionNotFound {
			t.Fatal("unconfigured source descriptor influenced result")
		}
	}
	wrong, _ := NewResolutionReference(KindBaseline, "r", "v1", d)
	if wrong.Kind() == ref.Kind() {
		t.Fatal("cross kind accepted")
	}
}

func TestSourceModesDoNotOverrideAuthoritativeSemantics(t *testing.T) {
	d := testDigest(t)
	ref, _ := NewResolutionReference(KindAuthorityRule, "r", "v1", d)
	a, _ := NewAuthoritySource("a", "memory", []ReferenceKind{KindAuthorityRule}, 1, d, true, SourceAuthoritative)
	m, _ := NewAuthoritySource("m", "mirror", []ReferenceKind{KindAuthorityRule}, 2, d, true, SourceMirror)
	f, _ := NewAuthoritySource("f", "fallback", []ReferenceKind{KindAuthorityRule}, 3, d, true, SourceFallback)
	cfg, _ := NewSourceConfiguration([]AuthoritySource{a, m, f}, d)
	obj, _ := NewResolvedObject(ref, d, []byte("exact"))
	other, _ := NewResolvedObject(ref, d, []byte("other"))
	// A mirror disagreement is diagnostic only; fallback is excluded when an
	// authoritative source exists.
	result := AggregateObservations(ref, cfg, []SourceObservation{{Source: a, State: ResolutionResolved, Object: obj}, {Source: m, State: ResolutionResolved, Object: other}, {Source: f, State: ResolutionResolved, Object: other}})
	if result.State() != ResolutionResolved {
		t.Fatalf("mirror/fallback changed authoritative result: %v", result.State())
	}
	// An authoritative exact result plus authoritative absence is not
	// definitive, even when the exact source has higher priority.
	b, _ := NewAuthoritySource("b", "memory", []ReferenceKind{KindAuthorityRule}, 2, d, true, SourceAuthoritative)
	cfg, _ = NewSourceConfiguration([]AuthoritySource{a, b}, d)
	if result = AggregateObservations(ref, cfg, []SourceObservation{{Source: a, State: ResolutionResolved, Object: obj}, {Source: b, State: ResolutionNotFound}}); result.State() != ResolutionUnavailable {
		t.Fatalf("exact plus authoritative absence = %v", result.State())
	}
}

func TestResolvedBundleAndFactsCopy(t *testing.T) {
	d := testDigest(t)
	ref, _ := NewResolutionReference(KindAuthorityRule, "r", "v1", d)
	s, _ := NewAuthoritySource("a", "memory", []ReferenceKind{KindAuthorityRule}, 1, d, true, SourceAuthoritative)
	obj, _ := NewResolvedObject(ref, d, []byte("x"))
	root, _ := NewRootTrustConfiguration(RootTrustConfiguration{id: "root", version: "1", digest: d, anchorIdentities: []string{"a"}})
	source := []byte("x")
	obj, _ = NewResolvedObject(ref, d, source)
	bundle, err := NewResolvedAuthorityBundle([]ResolvedObject{obj}, nil, d, root)
	if err != nil {
		t.Fatal(err)
	}
	source[0] = 'y'
	if !bytes.Equal(bundle.Objects()[0].Snapshot(), []byte("x")) {
		t.Fatal("bundle aliased source")
	}
	objects := bundle.Objects()
	objects[0].content[0] = 'z'
	if !bytes.Equal(bundle.Objects()[0].Snapshot(), []byte("x")) {
		t.Fatal("bundle getter exposed object alias")
	}
	f, err := NewFactFreshness(nil, time.Minute, "epoch", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = NewCurrentAuthorityFact(CurrentAuthorityFact{factKind: "revocation", subject: "r", source: "a", result: "NOT_REVOKED", observedAt: time.Unix(1, 0), freshness: f, verificationStatus: "VERIFIED"}); err != nil {
		t.Fatal(err)
	}
	_ = s
}

func TestRootAndVerifierValidation(t *testing.T) {
	d := testDigest(t)
	if _, err := NewRootTrustConfiguration(RootTrustConfiguration{id: "r", version: "1", digest: d}); err == nil {
		t.Fatal("root without anchor accepted")
	}
	if _, err := NewVerifierSemanticIdentity(VerifierSemanticIdentity{id: "v", version: "1", digest: d}); err == nil {
		t.Fatal("incomplete verifier accepted")
	}
	if _, err := NewFactFreshness(nil, 0, "", true); err != nil {
		t.Fatal(err)
	}
}
