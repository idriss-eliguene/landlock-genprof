//go:build envtest

package history

import (
	"context"
	"sync"
	"testing"
)

// The one-effect contract is a concurrent API-server protocol. Keep this
// test on envtest so resourceVersion/CAS behavior is exercised by Kubernetes
// rather than the in-memory dynamic fake, which does not model concurrent
// writes atomically.
func TestObservationAdapterConcurrentReplayHasOneEffect(t *testing.T) {
	client := setupEnvtest(t)
	observation := testVariantObservation(t, "observation-adapter-concurrent", "adapter-concurrent", "app", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "/tmp/concurrent", 9443)
	start := make(chan struct{})
	var wait sync.WaitGroup
	results := make([]ContributionApplyResult, 2)
	errors := make([]error, 2)
	for i := range results {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			<-start
			results[i], errors[i] = ApplyObservationContribution(context.Background(), client, "default", observation)
		}(i)
	}
	close(start)
	wait.Wait()
	for i, err := range errors {
		if err != nil {
			t.Fatalf("concurrent caller %d: %v", i, err)
		}
	}

	identity := PopulationIdentity{Scope: ScopeContainer, Target: "Deployment/adapter-concurrent", Container: "app", ImageIdentity: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	name, err := RecordNameContainerV2(identity)
	if err != nil {
		t.Fatal(err)
	}
	record, err := Get(context.Background(), client, "default", name)
	if err != nil || record == nil || len(record.Populations) != 1 || len(record.Populations[0].ObservationContributions) != 1 {
		t.Fatalf("concurrent effect = %#v, %v", record, err)
	}
	retry, err := ApplyObservationContribution(context.Background(), client, "default", observation)
	if err != nil || retry != ContributionAlreadyCommitted {
		t.Fatalf("retry after concurrent calls = %s, %v", retry, err)
	}
}
