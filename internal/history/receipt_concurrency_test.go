// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

package history

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// receiptConcurrencyCase builds an independent Contribution/history-name pair
// keyed by suffix, so HCON-5 can prove two distinct ContributionKeys never
// interfere with each other.
func receiptConcurrencyCase(t *testing.T, suffix string) (Contribution, string) {
	t.Helper()
	c := containerTestContribution()
	c.ObservationID = "observation-container-" + suffix
	c.Population.Container = "app-" + suffix
	name, err := RecordNameContainerV2(PopulationIdentity{Scope: ScopeContainer, Target: c.Population.Target, Container: c.Population.Container, ImageIdentity: c.Population.ImageIdentity})
	if err != nil {
		t.Fatal(err)
	}
	return c, name
}

// runConcurrent fires n ApplyContribution calls at the same client/case
// through a start barrier so they race as concurrently as the Go scheduler
// allows, mirroring the barrier pattern already used elsewhere in this
// package's concurrency tests (e.g. TestObservationAdapterConcurrentReplayHasOneEffect).
func runConcurrent(client *dynamicfake.FakeDynamicClient, c Contribution, n int) ([]ContributionApplyResult, []error) {
	start := make(chan struct{})
	var wait sync.WaitGroup
	results := make([]ContributionApplyResult, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		i := i
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			results[i], errs[i] = ApplyContribution(context.Background(), client, "default", c)
		}()
	}
	close(start)
	wait.Wait()
	return results, errs
}

// HCON-1, HCON-2, HCON-3, HCON-7, HCON-8: two concurrent ApplyContribution
// calls with an identical ContributionKey converge on exactly one durable
// history effect, neither caller observes ErrReceiptCommitFailure solely
// because the other committed first (this is the exact G8-DEFECT-01
// reproducer at the history-primitive layer), the final receipt is
// COMMITTED, no pending marker is left behind, and no contribution fact is
// lost.
func TestReceiptConcurrencySameKeyConvergesOnOneEffect(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	c, name := receiptConcurrencyCase(t, "same-key")

	results, errs := runConcurrent(client, c, 8)

	for i, err := range errs {
		if err != nil {
			t.Fatalf("HCON-2: goroutine %d unexpectedly errored: %v", i, err)
		}
	}
	applied, alreadyCommitted, recovered := 0, 0, 0
	for _, result := range results {
		switch result {
		case ContributionApplied:
			applied++
		case ContributionAlreadyCommitted:
			alreadyCommitted++
		case ContributionRecoveredAndCommitted:
			recovered++
		default:
			t.Fatalf("HCON-1: unexpected result %s", result)
		}
	}
	if applied+alreadyCommitted+recovered != len(results) {
		t.Fatalf("HCON-1: unexpected result mix: applied=%d alreadyCommitted=%d recovered=%d", applied, alreadyCommitted, recovered)
	}
	if applied > 1 {
		t.Fatalf("HCON-1: %d callers reported APPLIED, want at most 1 (one durable effect)", applied)
	}

	record, err := Get(context.Background(), client, "default", name)
	if err != nil || record == nil || len(record.Populations) != 1 {
		t.Fatalf("HCON-1: record = %#v, %v, want exactly one Population", record, err)
	}
	pop := record.Populations[0]
	if len(pop.ObservationContributions) != 1 {
		t.Fatalf("HCON-8: ObservationContributions = %#v, want exactly one — no contribution loss, no duplication", pop.ObservationContributions)
	}
	if pop.ObservationContributions[0].ObservationID != c.ObservationID {
		t.Fatalf("HCON-8: contribution lost or corrupted: %#v", pop.ObservationContributions[0])
	}
	if len(pop.PendingContributionMarkers) != 0 {
		t.Fatalf("HCON-7: %d pending markers remain, want 0 (fully cleaned up)", len(pop.PendingContributionMarkers))
	}

	key := ContributionKey{ObservationID: c.ObservationID, Population: PopulationFingerprint{Scope: ScopeContainer, Target: c.Population.Target, Container: c.Population.Container, ImageIdentity: c.Population.ImageIdentity}}
	receipts, err := NewReceiptStore(client)
	if err != nil {
		t.Fatal(err)
	}
	receipt, _, err := receipts.Get(context.Background(), "default", key)
	if err != nil || receipt == nil {
		t.Fatalf("HCON-3: receipt = %#v, %v", receipt, err)
	}
	if receipt.State != ReceiptCommitted {
		t.Fatalf("HCON-3: final receipt state = %s, want COMMITTED", receipt.State)
	}
}

// HCON-4: replaying ApplyContribution after the receipt is already COMMITTED
// returns the existing benign idempotent outcome, not an error.
func TestReceiptConcurrencyReplayAfterCommitIsBenign(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	c, _ := receiptConcurrencyCase(t, "replay")
	if result, err := ApplyContribution(context.Background(), client, "default", c); err != nil || result != ContributionApplied {
		t.Fatalf("initial apply = %s, %v", result, err)
	}
	result, err := ApplyContribution(context.Background(), client, "default", c)
	if err != nil {
		t.Fatalf("HCON-4: replay after commit returned an error: %v", err)
	}
	if result != ContributionAlreadyCommitted {
		t.Fatalf("HCON-4: replay after commit = %s, want ALREADY_COMMITTED", result)
	}
}

// HCON-5: concurrent callers using DIFFERENT ContributionKeys remain fully
// independent — no cross-key interference from the shared TrainingHistory
// record or the receipt store.
func TestReceiptConcurrencyDifferentKeysRemainIndependent(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	a, nameA := receiptConcurrencyCase(t, "independent-a")
	b, nameB := receiptConcurrencyCase(t, "independent-b")

	start := make(chan struct{})
	var wait sync.WaitGroup
	var errA, errB error
	var resultA, resultB ContributionApplyResult
	wait.Add(2)
	go func() { defer wait.Done(); <-start; resultA, errA = ApplyContribution(context.Background(), client, "default", a) }()
	go func() { defer wait.Done(); <-start; resultB, errB = ApplyContribution(context.Background(), client, "default", b) }()
	close(start)
	wait.Wait()

	if errA != nil || resultA != ContributionApplied {
		t.Fatalf("HCON-5: contribution A = %s, %v", resultA, errA)
	}
	if errB != nil || resultB != ContributionApplied {
		t.Fatalf("HCON-5: contribution B = %s, %v", resultB, errB)
	}
	recordA, err := Get(context.Background(), client, "default", nameA)
	if err != nil || recordA == nil || len(recordA.Populations) != 1 || len(recordA.Populations[0].ObservationContributions) != 1 {
		t.Fatalf("HCON-5: record A = %#v, %v", recordA, err)
	}
	recordB, err := Get(context.Background(), client, "default", nameB)
	if err != nil || recordB == nil || len(recordB.Populations) != 1 || len(recordB.Populations[0].ObservationContributions) != 1 {
		t.Fatalf("HCON-5: record B = %#v, %v", recordB, err)
	}
}

// HCON-6 / §12 mismatch negative tests: a COMMITTED receipt whose persisted
// identity or content differs from what the caller expects must remain a
// hard, fail-closed error — benign convergence is only for a truly
// equivalent concurrent caller, never for a genuine mismatch.
func TestReceiptCommitMismatchIsFailClosed(t *testing.T) {
	t.Run("different content digest", func(t *testing.T) {
		client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		key := provenanceKey()
		store, err := NewReceiptStore(client)
		if err != nil {
			t.Fatal(err)
		}
		digestA := strings.Repeat("a", 64)
		digestB := strings.Repeat("b", 64)
		_, rv, err := store.CreatePrepared(context.Background(), "default", key, "default", "history", digestA)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.Commit(context.Background(), "default", key, rv, digestA); err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.Commit(context.Background(), "default", key, "", digestB); err == nil {
			t.Fatal("HCON-6: committed receipt with a mismatching content digest was accepted as benign")
		} else if !errors.Is(err, ErrContributionContentMismatch) {
			t.Fatalf("HCON-6: error = %v, want ErrContributionContentMismatch", err)
		}
	})

	t.Run("wrong persisted identity under the requested receipt name", func(t *testing.T) {
		// Mirrors TestReceiptLookupRejectsWrongIdentityAndScope's forging
		// technique (a receipt persisted under the requested key's
		// deterministic name but carrying a DIFFERENT identity — the only
		// realistic way to exercise a name collision deterministically),
		// proving Commit's identity check — unchanged by this fix, but on
		// the exact code path under scrutiny — remains fail-closed.
		client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		requested := containerProvenanceKey()
		requestedName, err := requested.ReceiptName()
		if err != nil {
			t.Fatal(err)
		}
		wrong := requested
		wrong.Population.Target = "Deployment/wrong"
		wrongDigest, err := wrong.Digest()
		if err != nil {
			t.Fatal(err)
		}
		wrongReceipt := ObservationContributionReceipt{ObservationID: wrong.ObservationID, Population: wrong.Population, TrainingHistoryNamespace: "default", TrainingHistoryName: "history", ContributionKeyDigest: wrongDigest, State: ReceiptCommitted}
		if _, err := client.Resource(contributionReceiptGVR).Namespace("default").Create(context.Background(), receiptToUnstructured("default", requestedName, wrongReceipt), metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
		store, err := NewReceiptStore(client)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.Commit(context.Background(), "default", requested, "", "irrelevant"); !errors.Is(err, ErrReceiptIdentityMismatch) {
			t.Fatalf("HCON-6: Commit against a wrong-identity receipt = %v, want ErrReceiptIdentityMismatch", err)
		}
	})

	t.Run("different PopulationFingerprint produces an independent key, never a mismatch collision", func(t *testing.T) {
		// A different Population changes the receipt's deterministic name
		// entirely (ContributionKey.ReceiptName is derived from both
		// fields), so this is really HCON-5's independence guarantee
		// restated as a negative: it must never be misclassified as an
		// identity mismatch against an unrelated receipt.
		client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		a, _ := receiptConcurrencyCase(t, "mismatch-population-a")
		b, _ := receiptConcurrencyCase(t, "mismatch-population-b")
		if result, err := ApplyContribution(context.Background(), client, "default", a); err != nil || result != ContributionApplied {
			t.Fatalf("apply a = %s, %v", result, err)
		}
		if result, err := ApplyContribution(context.Background(), client, "default", b); err != nil || result != ContributionApplied {
			t.Fatalf("apply b = %s, %v", result, err)
		}
	})
}

// §11 exact commit-time TOCTOU reproducer: caller A and caller B both read
// the receipt as PREPARED; A commits; B's commit-time fresh read then
// observes COMMITTED. B must converge benignly, not receive
// ErrInvalidContribution/ErrReceiptCommitFailure. This is reproduced
// deterministically (not via goroutine timing) because ReceiptStore.Commit
// always performs its own fresh read internally, so two sequential Commit
// calls against the same originally-PREPARED resourceVersion exactly
// reproduce the interleaving without relying on scheduler luck.
func TestReceiptCommitTimeTOCTOUConvergesBenignly(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	key := provenanceKey()
	store, err := NewReceiptStore(client)
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("c", 64)

	// Both A and B observe PREPARED via CreatePrepared/Get before either
	// commits (simulated: a single prepare step both parties are said to
	// have already observed).
	_, rv, err := store.CreatePrepared(context.Background(), "default", key, "default", "history", digest)
	if err != nil {
		t.Fatal(err)
	}

	// A commits.
	committedByA, _, err := store.Commit(context.Background(), "default", key, rv, digest)
	if err != nil {
		t.Fatal(err)
	}
	if committedByA.State != ReceiptCommitted {
		t.Fatalf("A's commit did not reach COMMITTED: %#v", committedByA)
	}

	// B now performs its own commit-time fresh read using the SAME stale
	// resourceVersion it captured back when the receipt was still PREPARED,
	// and its own expected content digest (equivalent content, since B is
	// contributing the same observation/population).
	committedByB, _, err := store.Commit(context.Background(), "default", key, rv, digest)
	if err != nil {
		t.Fatalf("§11: B observed COMMITTED at commit-time and must converge benignly, got error: %v", err)
	}
	if committedByB.State != ReceiptCommitted {
		t.Fatalf("§11: B's benign-convergence result state = %s, want COMMITTED", committedByB.State)
	}
}
