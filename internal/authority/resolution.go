package authority

import (
	"bytes"
	"fmt"
	"sort"
	"time"
)

type ResolutionState uint8

const (
	ResolutionInvalid ResolutionState = iota
	ResolutionResolved
	ResolutionNotFound
	ResolutionUnavailable
	ResolutionMalformed
	ResolutionDigestMismatch
	ResolutionAmbiguous
	ResolutionTypeMismatch
	ResolutionVersionMismatch
)

type ReferenceKind string

const (
	KindAuthorityRule       ReferenceKind = "AuthorityRule"
	KindTrustPolicy         ReferenceKind = "TrustPolicy"
	KindBaseline            ReferenceKind = "Baseline"
	KindRegistry            ReferenceKind = "Registry"
	KindEvidence            ReferenceKind = "Evidence"
	KindVerifier            ReferenceKind = "Verifier"
	KindCertification       ReferenceKind = "Certification"
	KindCompatibilityRule   ReferenceKind = "CompatibilityRule"
	KindCompositionOperator ReferenceKind = "CompositionOperator"
)

type ResolutionReference struct {
	kind        ReferenceKind
	id, version string
	digest      Digest
}

func NewResolutionReference(kind ReferenceKind, id, version string, digest Digest) (ResolutionReference, error) {
	if kind == "" || id == "" || version == "" || !digest.Valid() {
		return ResolutionReference{}, fmt.Errorf("malformed exact resolution reference")
	}
	return ResolutionReference{kind: kind, id: id, version: version, digest: digest}, nil
}
func (r ResolutionReference) Kind() ReferenceKind { return r.kind }
func (r ResolutionReference) ID() string          { return r.id }
func (r ResolutionReference) Version() string     { return r.version }
func (r ResolutionReference) Digest() Digest      { return r.digest }

type SourceMode uint8

const (
	SourceModeInvalid SourceMode = iota
	SourceAuthoritative
	SourceMirror
	SourceFallback
)

type AuthoritySource struct {
	id, kind     string
	objectKinds  []ReferenceKind
	priority     int
	configDigest Digest
	enabled      bool
	mode         SourceMode
}

func NewAuthoritySource(id, kind string, objectKinds []ReferenceKind, priority int, configDigest Digest, enabled bool, mode SourceMode) (AuthoritySource, error) {
	if id == "" || kind == "" || len(objectKinds) == 0 || priority < 0 || !configDigest.Valid() || mode == SourceModeInvalid {
		return AuthoritySource{}, fmt.Errorf("invalid authority source")
	}
	cp := append([]ReferenceKind(nil), objectKinds...)
	return AuthoritySource{id: id, kind: kind, objectKinds: cp, priority: priority, configDigest: configDigest, enabled: enabled, mode: mode}, nil
}
func (s AuthoritySource) ID() string           { return s.id }
func (s AuthoritySource) Mode() SourceMode     { return s.mode }
func (s AuthoritySource) Priority() int        { return s.priority }
func (s AuthoritySource) ConfigDigest() Digest { return s.configDigest }
func (s AuthoritySource) Enabled() bool        { return s.enabled }
func (s AuthoritySource) Supports(k ReferenceKind) bool {
	for _, x := range s.objectKinds {
		if x == k {
			return true
		}
	}
	return false
}

type SourceConfiguration struct {
	sources []AuthoritySource
	digest  Digest
}

func NewSourceConfiguration(sources []AuthoritySource, digest Digest) (SourceConfiguration, error) {
	if len(sources) == 0 || !digest.Valid() {
		return SourceConfiguration{}, fmt.Errorf("invalid source configuration")
	}
	seen := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		if _, ok := seen[source.ID()]; ok {
			return SourceConfiguration{}, fmt.Errorf("duplicate authority source %q", source.ID())
		}
		seen[source.ID()] = struct{}{}
	}
	cp := append([]AuthoritySource(nil), sources...)
	return SourceConfiguration{sources: cp, digest: digest}, nil
}
func (c SourceConfiguration) Sources() []AuthoritySource {
	return append([]AuthoritySource(nil), c.sources...)
}
func (c SourceConfiguration) Digest() Digest { return c.digest }

type SourceObservation struct {
	Source AuthoritySource
	State  ResolutionState
	Object ResolvedObject
}
type ResolvedObject struct {
	reference     ResolutionReference
	contentDigest Digest
	content       []byte
}

func NewResolvedObject(ref ResolutionReference, contentDigest Digest, content []byte) (ResolvedObject, error) {
	if contentDigest != ref.Digest() || len(content) == 0 {
		return ResolvedObject{}, fmt.Errorf("invalid resolved object")
	}
	return ResolvedObject{reference: ref, contentDigest: contentDigest, content: append([]byte(nil), content...)}, nil
}
func (o ResolvedObject) Reference() ResolutionReference { return o.reference }
func (o ResolvedObject) ContentDigest() Digest          { return o.contentDigest }
func (o ResolvedObject) Snapshot() []byte               { return append([]byte(nil), o.content...) }

type ResolutionProvenance struct {
	sourceID            string
	mode                SourceMode
	priority            int
	configurationDigest Digest
	state               ResolutionState
}

func (p ResolutionProvenance) SourceID() string            { return p.sourceID }
func (p ResolutionProvenance) Mode() SourceMode            { return p.mode }
func (p ResolutionProvenance) Priority() int               { return p.priority }
func (p ResolutionProvenance) ConfigurationDigest() Digest { return p.configurationDigest }
func (p ResolutionProvenance) State() ResolutionState      { return p.state }

type ResolutionResult struct {
	state      ResolutionState
	object     *ResolvedObject
	provenance []ResolutionProvenance
}

func (r ResolutionResult) State() ResolutionState { return r.state }
func (r ResolutionResult) Object() *ResolvedObject {
	if r.object == nil {
		return nil
	}
	o := *r.object
	o.content = append([]byte(nil), o.content...)
	return &o
}
func (r ResolutionResult) Provenance() []ResolutionProvenance {
	return append([]ResolutionProvenance(nil), r.provenance...)
}

// AggregateObservations deterministically applies ADR-0012 source semantics
// to already-observed immutable snapshots. It performs no I/O or lookup.
func AggregateObservations(ref ResolutionReference, cfg SourceConfiguration, observations []SourceObservation) ResolutionResult {
	configured := make(map[string]AuthoritySource, len(cfg.sources))
	authoritativeConfigured := false
	for _, source := range cfg.sources {
		configured[source.ID()] = source
		if source.Enabled() && source.Supports(ref.Kind()) && source.Mode() == SourceAuthoritative {
			authoritativeConfigured = true
		}
	}
	prov := make([]ResolutionProvenance, 0, len(observations))
	var exact []ResolvedObject
	var conflict bool
	unavailable := false
	found := false
	notFound := false
	var soleNegative ResolutionState
	for _, o := range observations {
		configuredSource, configuredOK := configured[o.Source.ID()]
		if !configuredOK || !sameSourceDescriptor(configuredSource, o.Source) || !configuredSource.Enabled() || !configuredSource.Supports(ref.Kind()) {
			continue
		}
		if authoritativeConfigured && o.Source.Mode() == SourceFallback {
			continue
		}
		if !authoritativeConfigured && o.Source.Mode() == SourceMirror {
			continue
		}
		prov = append(prov, ResolutionProvenance{sourceID: o.Source.ID(), mode: o.Source.Mode(), priority: o.Source.Priority(), configurationDigest: o.Source.ConfigDigest(), state: o.State})
		if o.State == ResolutionUnavailable {
			if o.Source.Mode() == SourceAuthoritative {
				unavailable = true
			}
			continue
		}
		if o.State == ResolutionNotFound {
			if o.Source.Mode() == SourceAuthoritative {
				notFound = true
			}
			continue
		}
		if o.State != ResolutionResolved {
			if o.Source.Mode() == SourceMirror {
				continue
			}
			if soleNegative == ResolutionInvalid {
				soleNegative = o.State
			} else if soleNegative != o.State {
				conflict = true
			}
			continue
		}
		if o.Source.Mode() == SourceMirror {
			// Mirror observations are retained in provenance but cannot
			// override the authoritative snapshot in this pure aggregation
			// boundary. A later verifier may inspect the diagnostic.
			continue
		}
		if o.Object.Reference().Kind() != ref.Kind() || o.Object.Reference().ID() != ref.ID() {
			if o.Source.Mode() == SourceAuthoritative {
				if soleNegative == ResolutionInvalid {
					soleNegative = ResolutionTypeMismatch
				} else {
					conflict = true
				}
			}
			continue
		}
		if o.Object.Reference().Version() != ref.Version() {
			if o.Source.Mode() == SourceAuthoritative {
				if soleNegative == ResolutionInvalid {
					soleNegative = ResolutionVersionMismatch
				} else {
					conflict = true
				}
			}
			continue
		}
		if o.Object.ContentDigest() != ref.Digest() {
			if o.Source.Mode() == SourceAuthoritative {
				if soleNegative == ResolutionInvalid {
					soleNegative = ResolutionDigestMismatch
				} else {
					conflict = true
				}
			}
			continue
		}
		found = true
		exact = append(exact, o.Object)
	}
	sort.Slice(prov, func(i, j int) bool {
		if prov[i].SourceID() != prov[j].SourceID() {
			return prov[i].SourceID() < prov[j].SourceID()
		}
		return prov[i].Priority() < prov[j].Priority()
	})
	sort.Slice(exact, func(i, j int) bool { return bytes.Compare(exact[i].Snapshot(), exact[j].Snapshot()) < 0 })
	if conflict {
		return ResolutionResult{state: ResolutionAmbiguous, provenance: prov}
	}
	if unavailable {
		return ResolutionResult{state: ResolutionUnavailable, provenance: prov}
	}
	if notFound && found {
		return ResolutionResult{state: ResolutionUnavailable, provenance: prov}
	}
	if soleNegative != ResolutionInvalid && !found {
		return ResolutionResult{state: soleNegative, provenance: prov}
	}
	if found {
		if len(exact) == 0 {
			return ResolutionResult{state: ResolutionAmbiguous, provenance: prov}
		}
		for i := 1; i < len(exact); i++ {
			if !bytes.Equal(exact[0].Snapshot(), exact[i].Snapshot()) {
				return ResolutionResult{state: ResolutionAmbiguous, provenance: prov}
			}
		}
		obj := exact[0]
		return ResolutionResult{state: ResolutionResolved, object: &obj, provenance: prov}
	}
	return ResolutionResult{state: ResolutionNotFound, provenance: prov}
}

func sameSourceDescriptor(a, b AuthoritySource) bool {
	return a.ID() == b.ID() && a.kind == b.kind && a.Mode() == b.Mode() &&
		a.Priority() == b.Priority() && a.ConfigDigest() == b.ConfigDigest() &&
		a.Enabled() == b.Enabled()
}

type RootTrustConfiguration struct {
	id                string
	version           string
	digest            Digest
	anchorIdentities  []string
	applicableClasses []string
	provenance        string
}

func NewRootTrustConfiguration(v RootTrustConfiguration) (RootTrustConfiguration, error) {
	if v.id == "" || v.version == "" || !v.digest.Valid() || len(v.anchorIdentities) == 0 {
		return RootTrustConfiguration{}, fmt.Errorf("invalid root trust configuration")
	}
	v.anchorIdentities = append([]string(nil), v.anchorIdentities...)
	v.applicableClasses = append([]string(nil), v.applicableClasses...)
	return v, nil
}
func (r RootTrustConfiguration) ID() string      { return r.id }
func (r RootTrustConfiguration) Version() string { return r.version }
func (r RootTrustConfiguration) Digest() Digest  { return r.digest }
func (r RootTrustConfiguration) AnchorIdentities() []string {
	return append([]string(nil), r.anchorIdentities...)
}
func (r RootTrustConfiguration) ApplicableClasses() []string {
	return append([]string(nil), r.applicableClasses...)
}
func (r RootTrustConfiguration) Provenance() string { return r.provenance }

type VerifierBindingResult uint8

const (
	VerifierBindingInvalid VerifierBindingResult = iota
	VerifierBound
	VerifierMismatch
	VerifierUnknown
)

type VerifierSemanticIdentity struct {
	id           string
	version      string
	digest       Digest
	class        string
	inputSchema  string
	outputSchema string
	property     string
	procedure    string
	constraints  []string
}

func NewVerifierSemanticIdentity(v VerifierSemanticIdentity) (VerifierSemanticIdentity, error) {
	if v.id == "" || v.version == "" || !v.digest.Valid() || v.class == "" || v.inputSchema == "" || v.outputSchema == "" || v.property == "" || v.procedure == "" {
		return VerifierSemanticIdentity{}, fmt.Errorf("invalid verifier semantic identity")
	}
	v.constraints = append([]string(nil), v.constraints...)
	return v, nil
}
func (v VerifierSemanticIdentity) ID() string           { return v.id }
func (v VerifierSemanticIdentity) Version() string      { return v.version }
func (v VerifierSemanticIdentity) Digest() Digest       { return v.digest }
func (v VerifierSemanticIdentity) Class() string        { return v.class }
func (v VerifierSemanticIdentity) InputSchema() string  { return v.inputSchema }
func (v VerifierSemanticIdentity) OutputSchema() string { return v.outputSchema }
func (v VerifierSemanticIdentity) Property() string     { return v.property }
func (v VerifierSemanticIdentity) Procedure() string    { return v.procedure }
func (v VerifierSemanticIdentity) Constraints() []string {
	return append([]string(nil), v.constraints...)
}

type FactFreshness struct {
	validUntil  *time.Time
	maxAge      time.Duration
	sourceEpoch string
	nonExpiring bool
}

func NewFactFreshness(validUntil *time.Time, maxAge time.Duration, epoch string, nonExpiring bool) (FactFreshness, error) {
	if maxAge < 0 || (validUntil != nil && validUntil.IsZero()) || (nonExpiring && (validUntil != nil || maxAge != 0)) {
		return FactFreshness{}, fmt.Errorf("invalid fact freshness")
	}
	var u *time.Time
	if validUntil != nil {
		t := *validUntil
		u = &t
	}
	return FactFreshness{validUntil: u, maxAge: maxAge, sourceEpoch: epoch, nonExpiring: nonExpiring}, nil
}
func (f FactFreshness) ValidUntil() *time.Time {
	if f.validUntil == nil {
		return nil
	}
	t := *f.validUntil
	return &t
}
func (f FactFreshness) MaxAge() time.Duration { return f.maxAge }
func (f FactFreshness) SourceEpoch() string   { return f.sourceEpoch }
func (f FactFreshness) NonExpiring() bool     { return f.nonExpiring }

type CurrentAuthorityFact struct {
	factKind           string
	subject            string
	source             string
	result             string
	observedAt         time.Time
	freshness          FactFreshness
	verificationStatus string
	sourceEpoch        string
}

func NewCurrentAuthorityFact(v CurrentAuthorityFact) (CurrentAuthorityFact, error) {
	if v.factKind == "" || v.subject == "" || v.source == "" || v.result == "" || v.observedAt.IsZero() || v.verificationStatus == "" {
		return CurrentAuthorityFact{}, fmt.Errorf("invalid current authority fact")
	}
	return v, nil
}
func (v CurrentAuthorityFact) FactKind() string           { return v.factKind }
func (v CurrentAuthorityFact) Subject() string            { return v.subject }
func (v CurrentAuthorityFact) Source() string             { return v.source }
func (v CurrentAuthorityFact) Result() string             { return v.result }
func (v CurrentAuthorityFact) ObservedAt() time.Time      { return v.observedAt }
func (v CurrentAuthorityFact) Freshness() FactFreshness   { return v.freshness }
func (v CurrentAuthorityFact) VerificationStatus() string { return v.verificationStatus }
func (v CurrentAuthorityFact) SourceEpoch() string        { return v.sourceEpoch }

type ResolvedAuthorityBundle struct {
	objects                   []ResolvedObject
	provenance                []ResolutionProvenance
	sourceConfigurationDigest Digest
	rootTrust                 RootTrustConfiguration
}

func NewResolvedAuthorityBundle(objects []ResolvedObject, prov []ResolutionProvenance, sourceDigest Digest, root RootTrustConfiguration) (ResolvedAuthorityBundle, error) {
	if !sourceDigest.Valid() {
		return ResolvedAuthorityBundle{}, fmt.Errorf("invalid bundle source configuration")
	}
	r, err := NewRootTrustConfiguration(root)
	if err != nil {
		return ResolvedAuthorityBundle{}, err
	}
	cp := append([]ResolvedObject(nil), objects...)
	for i := range cp {
		cp[i].content = append([]byte(nil), cp[i].content...)
	}
	pp := append([]ResolutionProvenance(nil), prov...)
	return ResolvedAuthorityBundle{objects: cp, provenance: pp, sourceConfigurationDigest: sourceDigest, rootTrust: r}, nil
}
func (b ResolvedAuthorityBundle) Objects() []ResolvedObject {
	cp := append([]ResolvedObject(nil), b.objects...)
	for i := range cp {
		cp[i].content = append([]byte(nil), cp[i].content...)
	}
	return cp
}
func (b ResolvedAuthorityBundle) Provenance() []ResolutionProvenance {
	return append([]ResolutionProvenance(nil), b.provenance...)
}
func (b ResolvedAuthorityBundle) SourceConfigurationDigest() Digest {
	return b.sourceConfigurationDigest
}
func (b ResolvedAuthorityBundle) RootTrust() RootTrustConfiguration { return b.rootTrust }
