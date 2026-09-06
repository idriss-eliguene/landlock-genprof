package history

import (
	"context"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func provenanceKey() ContributionKey {
	return ContributionKey{ObservationID: "observation-1", Population: PopulationFingerprint{Target: "Deployment/api", Container: "app", ImageIdentity: "sha256:image", BinaryPath: "/app/server"}}
}

func TestContributionKeyIsVersionedLengthPrefixedAndDeterministic(t *testing.T) {
	key := provenanceKey()
	a, err := key.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	b, err := key.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) || string(a[:19]) != "contribution-key-v1" {
		t.Fatalf("canonical encoding = %q", a)
	}
	other := key
	other.ObservationID = "observation-10"
	if reflect.DeepEqual(a, mustBytes(t, other)) {
		t.Fatal("distinct key fields collided")
	}
	name, err := key.ReceiptName()
	if err != nil {
		t.Fatal(err)
	}
	if len(name) != len("obscontrib-")+32 {
		t.Fatalf("receipt name = %s", name)
	}
}

func mustBytes(t *testing.T, key ContributionKey) []byte {
	t.Helper()
	value, err := key.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestContributionMetadataValidation(t *testing.T) {
	key := provenanceKey()
	digest, _ := key.Digest()
	population := Population{Scope: ScopeBinary, Target: key.Population.Target, Container: key.Population.Container, ImageIdentity: key.Population.ImageIdentity, BinaryPath: key.Population.BinaryPath, ObservationContributions: []ObservationContribution{{ObservationID: key.ObservationID, Sources: []ObservationSourceContribution{{Source: "filesystem", EvidenceState: "UNKNOWN", AttributionState: "COMPLETED", AttributedCount: 2, ExcludedCount: 1, NormalizedFactCount: 1}}}}, PendingContributionMarkers: []ContributionMarker{{ObservationID: key.ObservationID, Population: key.Population, KeyDigest: digest}}}
	if err := population.ValidateObservationMetadata(); err != nil {
		t.Fatal(err)
	}
	duplicate := population
	duplicate.ObservationContributions = append(append([]ObservationContribution(nil), population.ObservationContributions...), population.ObservationContributions[0])
	if err := duplicate.ValidateObservationMetadata(); err == nil {
		t.Fatal("duplicate observation contribution accepted")
	}
	bad := population
	bad.PendingContributionMarkers = []ContributionMarker{{ObservationID: key.ObservationID, Population: key.Population, KeyDigest: "bad"}}
	if err := bad.ValidateObservationMetadata(); err == nil {
		t.Fatal("invalid marker accepted")
	}
	if !(PopulationFingerprint{Target: key.Population.Target, Container: key.Population.Container, ImageIdentity: key.Population.ImageIdentity, BinaryPath: key.Population.BinaryPath}).Valid() {
		t.Fatal("population fingerprint semantics changed")
	}
}

func TestReceiptStateAndIdentityValidation(t *testing.T) {
	key := provenanceKey()
	digest, _ := key.Digest()
	receipt := ObservationContributionReceipt{ObservationID: key.ObservationID, Population: key.Population, TrainingHistoryNamespace: "default", TrainingHistoryName: "history", ContributionKeyDigest: digest, State: ReceiptPrepared}
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}
	receipt.State = ReceiptCommitted
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}
	receipt.ContributionKeyDigest = "0000000000000000000000000000000000000000000000000000000000000000"
	if err := receipt.Validate(); err == nil {
		t.Fatal("receipt key mismatch accepted")
	}
}

func TestObservationMetadataRoundTrip(t *testing.T) {
	key := provenanceKey()
	digest, err := key.Digest()
	if err != nil {
		t.Fatal(err)
	}
	record := &Record{Populations: []Population{{Scope: ScopeBinary, Target: key.Population.Target, Container: key.Population.Container, ImageIdentity: key.Population.ImageIdentity, BinaryPath: key.Population.BinaryPath, ObservationContributions: []ObservationContribution{{ObservationID: key.ObservationID, Sources: []ObservationSourceContribution{{Source: string(SourceExec), EvidenceState: "UNKNOWN", AttributionState: "COMPLETED", AttributedCount: 1}}}}, PendingContributionMarkers: []ContributionMarker{{ObservationID: key.ObservationID, Population: key.Population, KeyDigest: digest}}}}}
	decoded, err := fromUnstructured(toUnstructured("default", "history", record))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.Populations, record.Populations) {
		t.Fatalf("metadata changed during round-trip: %#v", decoded.Populations)
	}
}

func TestReceiptStoreCreateGetAndCommit(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	store, err := NewReceiptStore(client)
	if err != nil {
		t.Fatal(err)
	}
	key := provenanceKey()
	created, rv, err := store.CreatePrepared(context.Background(), "default", key, "default", "history", "")
	if err != nil {
		t.Fatal(err)
	}
	if created.State != ReceiptPrepared {
		t.Fatalf("state = %s", created.State)
	}
	fetched, fetchedRV, err := store.Get(context.Background(), "default", key)
	if err != nil || fetched == nil {
		t.Fatalf("get = %#v, %v", fetched, err)
	}
	if fetched.ContributionKeyDigest != created.ContributionKeyDigest || fetchedRV != rv {
		t.Fatalf("get changed receipt identity or resource version")
	}
	committed, _, err := store.Commit(context.Background(), "default", key, rv)
	if err != nil {
		t.Fatal(err)
	}
	if committed.State != ReceiptCommitted {
		t.Fatalf("state = %s", committed.State)
	}
	if _, _, err := store.Commit(context.Background(), "default", key, ""); err == nil {
		t.Fatal("committed receipt was committed again")
	}
	_, _, err = store.CreatePrepared(context.Background(), "default", key, "default", "history", "")
	if err == nil {
		t.Fatal("duplicate receipt create unexpectedly succeeded")
	}
}
