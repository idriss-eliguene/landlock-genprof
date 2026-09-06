package history

import (
	"context"
	"errors"
	"sync"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

func testContribution() Contribution {
	return Contribution{
		ObservationID:  "observation-1",
		Population:     PopulationFingerprint{Target: "Deployment/api", Container: "app", ImageIdentity: "sha256:image", BinaryPath: "/app/server"},
		Filesystem:     []profile.FileAccess{{Path: "/etc/hosts", Permissions: []profile.FilePermission{profile.PermissionWrite, profile.PermissionRead}}, {Path: "/etc/hosts", Permissions: []profile.FilePermission{profile.PermissionExecute}}},
		NetworkConnect: []profile.NetworkAccess{{Port: 443, Direction: profile.DirectionEgress}, {Port: 443, Direction: profile.DirectionEgress}},
		Capabilities:   []profile.CapabilityAccess{{Name: "CAP_NET_RAW"}, {Name: "CAP_NET_RAW"}},
		Sources:        []ObservationSourceContribution{{Source: "filesystem", EvidenceState: "UNKNOWN", AttributionState: "COMPLETED", AttributedCount: 2, ExcludedCount: 1, NormalizedFactCount: 1}, {Source: "exec", EvidenceState: "UNKNOWN", AttributionState: "COMPLETED", AttributedCount: 1}},
	}
}

func containerTestContribution() Contribution {
	c := testContribution()
	c.ObservationID = "observation-container-1"
	c.Population = PopulationFingerprint{Scope: ScopeContainer, Target: "Deployment/api", Container: "app", ImageIdentity: "sha256:image"}
	return c
}

func TestApplyContainerContributionIsIdempotentAndScopeSeparated(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	contribution := containerTestContribution()
	result, err := ApplyContribution(context.Background(), client, "default", contribution)
	if err != nil || result != ContributionApplied {
		t.Fatalf("first apply = %s, %v", result, err)
	}
	result, err = ApplyContribution(context.Background(), client, "default", contribution)
	if err != nil || result != ContributionAlreadyCommitted {
		t.Fatalf("replay = %s, %v", result, err)
	}
	name, err := RecordNameContainerV2(PopulationIdentity{Scope: ScopeContainer, Target: contribution.Population.Target, Container: contribution.Population.Container, ImageIdentity: contribution.Population.ImageIdentity})
	if err != nil {
		t.Fatal(err)
	}
	record, err := Get(context.Background(), client, "default", name)
	if err != nil || record == nil || len(record.Populations) != 1 {
		t.Fatalf("container history = %#v, %v", record, err)
	}
	population := record.Populations[0]
	if population.Scope != ScopeContainer || population.BinaryPath != "" || population.RunsRecorded != 0 || len(population.ObservationContributions) != 1 || len(population.PendingContributionMarkers) != 0 {
		t.Fatalf("container contribution semantics = %#v", population)
	}
}

func TestContainerContributionCrashRecoveryMatrix(t *testing.T) {
	newCase := func() (Contribution, string) {
		c := containerTestContribution()
		name, err := RecordNameContainerV2(PopulationIdentity{Scope: ScopeContainer, Target: c.Population.Target, Container: c.Population.Container, ImageIdentity: c.Population.ImageIdentity})
		if err != nil {
			t.Fatal(err)
		}
		return c, name
	}
	applyOnce := func(t *testing.T, client dynamic.Interface, c Contribution) *Record {
		t.Helper()
		result, err := ApplyContribution(context.Background(), client, "default", c)
		if err != nil || result != ContributionApplied {
			t.Fatalf("apply = %s, %v", result, err)
		}
		_, name := newCase()
		record, err := Get(context.Background(), client, "default", name)
		if err != nil || record == nil || len(record.Populations) != 1 {
			t.Fatalf("record = %#v, %v", record, err)
		}
		return record
	}

	t.Run("C1-C2-C6-C7", func(t *testing.T) {
		client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		c, _ := newCase()
		first := applyOnce(t, client, c)
		second, err := ApplyContribution(context.Background(), client, "default", c)
		if err != nil || second != ContributionAlreadyCommitted {
			t.Fatalf("committed replay = %s, %v", second, err)
		}
		if len(first.Populations[0].ObservationContributions) != 1 || first.Populations[0].RunsRecorded != 0 {
			t.Fatalf("statistical firewall/effect count = %#v", first.Populations[0])
		}

		client = dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		c, name := newCase()
		normalized, err := c.normalize()
		if err != nil {
			t.Fatal(err)
		}
		key := ContributionKey{ObservationID: c.ObservationID, Population: normalized.Population}
		receipts, err := NewReceiptStore(client)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := receipts.CreatePrepared(context.Background(), "default", key, "default", name, mustDigest(t, c)); err != nil {
			t.Fatal(err)
		}
		result, err := ApplyContribution(context.Background(), client, "default", c)
		if err != nil || result != ContributionApplied {
			t.Fatalf("prepared recovery = %s, %v", result, err)
		}
	})

	t.Run("C3-conflict", func(t *testing.T) {
		underlying := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		client := newConflictInjectingClient(underlying, 1)
		c, _ := newCase()
		applyOnce(t, client, c)
	})

	t.Run("C4-C5", func(t *testing.T) {
		client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		c, name := newCase()
		normalized, err := c.normalize()
		if err != nil {
			t.Fatal(err)
		}
		key := ContributionKey{ObservationID: c.ObservationID, Population: normalized.Population}
		receipts, err := NewReceiptStore(client)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := receipts.CreatePrepared(context.Background(), "default", key, "default", name, mustDigest(t, c)); err != nil {
			t.Fatal(err)
		}
		keyDigest, err := key.Digest()
		if err != nil {
			t.Fatal(err)
		}
		marker := ContributionMarker{ObservationID: c.ObservationID, Population: normalized.Population, KeyDigest: keyDigest}
		if _, applied, err := applyHistoryEffect(context.Background(), client, "default", name, key, normalized, marker); err != nil || !applied {
			t.Fatalf("durable history effect = %t, %v", applied, err)
		}
		result, err := ApplyContribution(context.Background(), client, "default", c)
		if err != nil || result != ContributionRecoveredAndCommitted {
			t.Fatalf("prepared marker recovery = %s, %v", result, err)
		}
		client = dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		c, name = newCase()
		normalized, err = c.normalize()
		if err != nil {
			t.Fatal(err)
		}
		key = ContributionKey{ObservationID: c.ObservationID, Population: normalized.Population}
		receipts, err = NewReceiptStore(client)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := receipts.CreatePrepared(context.Background(), "default", key, "default", name, mustDigest(t, c)); err != nil {
			t.Fatal(err)
		}
		keyDigest, err = key.Digest()
		if err != nil {
			t.Fatal(err)
		}
		marker = ContributionMarker{ObservationID: c.ObservationID, Population: normalized.Population, KeyDigest: keyDigest}
		if _, applied, err := applyHistoryEffect(context.Background(), client, "default", name, key, normalized, marker); err != nil || !applied {
			t.Fatalf("committed-state history effect = %t, %v", applied, err)
		}
		receipt, rv, err := receipts.Get(context.Background(), "default", key)
		if err != nil || receipt == nil {
			t.Fatalf("prepared receipt = %#v, %v", receipt, err)
		}
		if _, _, err := receipts.Commit(context.Background(), "default", key, rv); err != nil {
			t.Fatal(err)
		}
		result, err = ApplyContribution(context.Background(), client, "default", c)
		if err != nil || result != ContributionAlreadyCommitted {
			t.Fatalf("committed marker replay = %s, %v", result, err)
		}
		if err := cleanupContributionMarker(context.Background(), client, "default", name, key, marker); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("C8-concurrent-duplicate", func(t *testing.T) {
		client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		c, name := newCase()
		results := make(chan ContributionApplyResult, 2)
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				result, err := ApplyContribution(context.Background(), client, "default", c)
				results <- result
				errs <- err
			}()
		}
		wg.Wait()
		close(results)
		close(errs)
		applied := 0
		for result := range results {
			if result == ContributionApplied {
				applied++
			}
		}
		for err := range errs {
			if err != nil && !errors.Is(err, ErrReceiptCommitFailure) {
				t.Fatalf("concurrent contribution = %v", err)
			}
		}
		if applied > 1 {
			t.Fatalf("duplicate effects reported: %d", applied)
		}
		record, err := Get(context.Background(), client, "default", name)
		if err != nil || record == nil || len(record.Populations) != 1 || len(record.Populations[0].ObservationContributions) != 1 {
			t.Fatalf("concurrent history effect = %#v, %v", record, err)
		}
	})
}

func mustDigest(t *testing.T, c Contribution) string {
	t.Helper()
	digest, err := ContributionDigest(c)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func TestApplyContributionIsIdempotentAndLeavesCountersUnchanged(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	contribution := testContribution()
	result, err := ApplyContribution(context.Background(), client, "default", contribution)
	if err != nil || result != ContributionApplied {
		t.Fatalf("first apply = %s, %v", result, err)
	}
	result, err = ApplyContribution(context.Background(), client, "default", contribution)
	if err != nil || result != ContributionAlreadyCommitted {
		t.Fatalf("replay = %s, %v", result, err)
	}
	name := RecordNameV2(contribution.Population.Container, contribution.Population.BinaryPath)
	record, err := Get(context.Background(), client, "default", name)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Populations) != 1 || record.Populations[0].RunsRecorded != 0 || len(record.Populations[0].FilesystemAccesses) != 1 || len(record.Populations[0].CapabilityAccesses) != 1 || len(record.Populations[0].ObservationContributions) != 1 || len(record.Populations[0].PendingContributionMarkers) != 0 {
		t.Fatalf("history effect = %#v", record)
	}
	if len(record.Populations[0].NetworkAccesses) != 1 || len(record.Populations[0].ObservationContributions[0].Sources) != 2 {
		t.Fatalf("deduplication/provenance = %#v", record.Populations[0])
	}
}

func TestApplyContributionDifferentObservationsPreserveFactsAndProvenance(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	a := testContribution()
	if _, err := ApplyContribution(context.Background(), client, "default", a); err != nil {
		t.Fatal(err)
	}
	b := testContribution()
	b.ObservationID = "observation-2"
	b.Filesystem = []profile.FileAccess{{Path: "/var/log/app", Permissions: []profile.FilePermission{profile.PermissionRead}}}
	b.NetworkConnect = nil
	b.Capabilities = nil
	b.Sources = []ObservationSourceContribution{{Source: "filesystem", EvidenceState: "AVAILABLE", AttributionState: "COMPLETED", AttributedCount: 1, NormalizedFactCount: 1}}
	if _, err := ApplyContribution(context.Background(), client, "default", b); err != nil {
		t.Fatal(err)
	}
	name := RecordNameV2(a.Population.Container, a.Population.BinaryPath)
	record, err := Get(context.Background(), client, "default", name)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Populations[0].ObservationContributions) != 2 || len(record.Populations[0].FilesystemAccesses) != 2 || record.Populations[0].RunsRecorded != 0 {
		t.Fatalf("merged contributions = %#v", record.Populations[0])
	}
}

func TestApplyContributionRejectsContentMismatch(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	a := testContribution()
	if _, err := ApplyContribution(context.Background(), client, "default", a); err != nil {
		t.Fatal(err)
	}
	b := a
	b.Filesystem = []profile.FileAccess{{Path: "/different", Permissions: []profile.FilePermission{profile.PermissionRead}}}
	if _, err := ApplyContribution(context.Background(), client, "default", b); !errors.Is(err, ErrContributionContentMismatch) {
		t.Fatalf("mismatch error = %v", err)
	}
}

func TestApplyContributionRecoversPreparedMarker(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	c := testContribution()
	normalized, err := c.normalize()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := normalized.contentDigest()
	if err != nil {
		t.Fatal(err)
	}
	key := ContributionKey{ObservationID: c.ObservationID, Population: c.Population}
	receipts, err := NewReceiptStore(client)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := receipts.CreatePrepared(context.Background(), "default", key, "default", RecordNameV2(key.Population.Container, key.Population.BinaryPath), digest); err != nil {
		t.Fatal(err)
	}
	keyDigest, err := key.Digest()
	if err != nil {
		t.Fatal(err)
	}
	marker := ContributionMarker{ObservationID: c.ObservationID, Population: c.Population, KeyDigest: keyDigest}
	if _, applied, err := applyHistoryEffect(context.Background(), client, "default", RecordNameV2(key.Population.Container, key.Population.BinaryPath), key, normalized, marker); err != nil || !applied {
		t.Fatalf("seed history effect = %t, %v", applied, err)
	}
	result, err := ApplyContribution(context.Background(), client, "default", c)
	if err != nil || result != ContributionRecoveredAndCommitted {
		t.Fatalf("recovery = %s, %v", result, err)
	}
}

func TestApplyContributionDoesNotCreateExecBehaviorFact(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	c := testContribution()
	c.Filesystem = nil
	c.NetworkConnect = nil
	c.Capabilities = nil
	if _, err := ApplyContribution(context.Background(), client, "default", c); err != nil {
		t.Fatal(err)
	}
	record, err := Get(context.Background(), client, "default", RecordNameV2(c.Population.Container, c.Population.BinaryPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Populations[0].SyscallAccesses) != 0 || len(record.Populations[0].ObservationContributions) != 1 {
		t.Fatalf("exec was mapped to behavior: %#v", record.Populations[0])
	}
}
