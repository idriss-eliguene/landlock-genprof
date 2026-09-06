package history

import (
	"context"
	"errors"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// prepareMarkerlessCommittedState deterministically reproduces the DEFECT-02
// window: the durable effect exists, its marker has been cleaned, and the
// authoritative receipt proves that this exact contribution committed.
func prepareMarkerlessCommittedState(t *testing.T, client *dynamicfake.FakeDynamicClient, c Contribution) (Contribution, string, ContributionMarker) {
	t.Helper()
	normalized, err := c.normalize()
	if err != nil {
		t.Fatal(err)
	}
	key := ContributionKey{ObservationID: c.ObservationID, Population: normalized.Population}
	historyName, err := resolveContributionHistoryName(context.Background(), client, "default", normalized.Population)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := normalized.contentDigest()
	if err != nil {
		t.Fatal(err)
	}
	receipts, err := NewReceiptStore(client)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := receipts.CreatePrepared(context.Background(), "default", key, "default", historyName, digest); err != nil {
		t.Fatal(err)
	}
	keyDigest, err := key.Digest()
	if err != nil {
		t.Fatal(err)
	}
	marker := ContributionMarker{ObservationID: c.ObservationID, Population: normalized.Population, KeyDigest: keyDigest}
	if _, applied, err := applyHistoryEffect(context.Background(), client, "default", historyName, key, normalized, marker); err != nil || !applied {
		t.Fatalf("initial effect = applied %t, err %v", applied, err)
	}
	if err := receiptsCommitForTest(t, client, key, digest); err != nil {
		t.Fatal(err)
	}
	if err := cleanupContributionMarker(context.Background(), client, "default", historyName, key, marker); err != nil {
		t.Fatal(err)
	}
	return c, historyName, marker
}

func receiptsCommitForTest(t *testing.T, client *dynamicfake.FakeDynamicClient, key ContributionKey, digest string) error {
	t.Helper()
	receipts, err := NewReceiptStore(client)
	if err != nil {
		return err
	}
	_, rv, err := receipts.Get(context.Background(), "default", key)
	if err != nil {
		return err
	}
	_, _, err = receipts.Commit(context.Background(), "default", key, rv, digest)
	return err
}

func TestApplyHistoryEffectMarkerlessProvenanceConvergesOnExactCommittedReceipt(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	c := containerTestContribution()
	_, name, marker := prepareMarkerlessCommittedState(t, client, c)
	normalized, err := c.normalize()
	if err != nil {
		t.Fatal(err)
	}
	key := ContributionKey{ObservationID: c.ObservationID, Population: normalized.Population}
	markerPresent, applied, err := applyHistoryEffect(context.Background(), client, "default", name, key, normalized, marker)
	if err != nil {
		t.Fatalf("equivalent markerless replay: %v", err)
	}
	if !markerPresent || applied {
		t.Fatalf("convergence result marker=%t applied=%t", markerPresent, applied)
	}
}

func TestApplyHistoryEffectMarkerlessProvenanceRequiresExactCommittedReceipt(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, *dynamicfake.FakeDynamicClient, Contribution, string, ContributionMarker)
		want  error
	}{
		{"N1 receipt absent", func(t *testing.T, client *dynamicfake.FakeDynamicClient, c Contribution, name string, marker ContributionMarker) {
		}, ErrInvalidContribution},
		{"N2 receipt prepared", func(t *testing.T, client *dynamicfake.FakeDynamicClient, c Contribution, name string, marker ContributionMarker) {
			normalized, _ := c.normalize()
			key := ContributionKey{ObservationID: c.ObservationID, Population: normalized.Population}
			digest, _ := normalized.contentDigest()
			receipts, err := NewReceiptStore(client)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := receipts.CreatePrepared(context.Background(), "default", key, "default", name, digest); err != nil {
				t.Fatal(err)
			}
		}, ErrInvalidContribution},
		{"N3 committed observation mismatch", func(t *testing.T, client *dynamicfake.FakeDynamicClient, c Contribution, name string, marker ContributionMarker) {
			normalized, _ := c.normalize()
			key := ContributionKey{ObservationID: c.ObservationID, Population: normalized.Population}
			wrong := key
			wrong.ObservationID = "other-observation"
			digest, _ := normalized.contentDigest()
			if _, err := client.Resource(contributionReceiptGVR).Namespace("default").Create(context.Background(), receiptToUnstructured("default", mustReceiptName(t, key), ObservationContributionReceipt{ObservationID: wrong.ObservationID, Population: wrong.Population, TrainingHistoryNamespace: "default", TrainingHistoryName: name, ContributionKeyDigest: mustKeyDigest(t, wrong), ContentDigest: digest, State: ReceiptCommitted}), metav1.CreateOptions{}); err != nil {
				t.Fatal(err)
			}
		}, ErrReceiptIdentityMismatch},
		{"N4 committed population mismatch", func(t *testing.T, client *dynamicfake.FakeDynamicClient, c Contribution, name string, marker ContributionMarker) {
			normalized, _ := c.normalize()
			key := ContributionKey{ObservationID: c.ObservationID, Population: normalized.Population}
			wrong := key
			wrong.Population.Container = "other-container"
			digest, _ := normalized.contentDigest()
			if _, err := client.Resource(contributionReceiptGVR).Namespace("default").Create(context.Background(), receiptToUnstructured("default", mustReceiptName(t, key), ObservationContributionReceipt{ObservationID: wrong.ObservationID, Population: wrong.Population, TrainingHistoryNamespace: "default", TrainingHistoryName: name, ContributionKeyDigest: mustKeyDigest(t, wrong), ContentDigest: digest, State: ReceiptCommitted}), metav1.CreateOptions{}); err != nil {
				t.Fatal(err)
			}
		}, ErrReceiptIdentityMismatch},
		{"N5 committed content mismatch", func(t *testing.T, client *dynamicfake.FakeDynamicClient, c Contribution, name string, marker ContributionMarker) {
			normalized, _ := c.normalize()
			key := ContributionKey{ObservationID: c.ObservationID, Population: normalized.Population}
			if _, err := client.Resource(contributionReceiptGVR).Namespace("default").Create(context.Background(), receiptToUnstructured("default", mustReceiptName(t, key), ObservationContributionReceipt{ObservationID: key.ObservationID, Population: key.Population, TrainingHistoryNamespace: "default", TrainingHistoryName: name, ContributionKeyDigest: mustKeyDigest(t, key), ContentDigest: strings.Repeat("f", 64), State: ReceiptCommitted}), metav1.CreateOptions{}); err != nil {
				t.Fatal(err)
			}
		}, ErrContributionContentMismatch},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
			c := containerTestContribution()
			normalized, err := c.normalize()
			if err != nil {
				t.Fatal(err)
			}
			key := ContributionKey{ObservationID: c.ObservationID, Population: normalized.Population}
			name, err := resolveContributionHistoryName(context.Background(), client, "default", normalized.Population)
			if err != nil {
				t.Fatal(err)
			}
			keyDigest, err := key.Digest()
			if err != nil {
				t.Fatal(err)
			}
			marker := ContributionMarker{ObservationID: c.ObservationID, Population: normalized.Population, KeyDigest: keyDigest}
			if _, applied, err := applyHistoryEffect(context.Background(), client, "default", name, key, normalized, marker); err != nil || !applied {
				t.Fatalf("seed effect = %t, %v", applied, err)
			}
			if err := cleanupContributionMarker(context.Background(), client, "default", name, key, marker); err != nil {
				t.Fatal(err)
			}
			tc.setup(t, client, c, name, marker)
			_, _, err = applyHistoryEffect(context.Background(), client, "default", name, key, normalized, marker)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func mustReceiptName(t *testing.T, key ContributionKey) string {
	t.Helper()
	name, err := key.ReceiptName()
	if err != nil {
		t.Fatal(err)
	}
	return name
}
func mustKeyDigest(t *testing.T, key ContributionKey) string {
	t.Helper()
	digest, err := key.Digest()
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
