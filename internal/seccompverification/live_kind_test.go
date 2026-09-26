package seccompverification

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// TestLiveKindTwinPodRuntimeVerification is opt-in and is enabled only by
// disposable Linux CI after SPO has materialized the supplied Localhost
// profile and the digest-pinned probe image has been loaded into Kind.
func TestLiveKindTwinPodRuntimeVerification(t *testing.T) {
	if os.Getenv("LANDLOCK_GENPROF_LIVE_KIND_VERIFICATION") != "1" {
		t.Skip("set by the disposable Kind integration workflow only")
	}
	get := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			t.Fatalf("missing %s", key)
		}
		return v
	}
	config, err := clientcmd.BuildConfigFromFlags("", get("LANDLOCK_GENPROF_VERIFIER_KUBECONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	namespace, node := get("LANDLOCK_GENPROF_VERIFIER_NAMESPACE"), get("LANDLOCK_GENPROF_VERIFIER_NODE")
	image, profile := get("LANDLOCK_GENPROF_VERIFIER_IMAGE"), get("LANDLOCK_GENPROF_VERIFIER_PROFILE")
	pods, err := NewScopedPodLifecycle(client, namespace)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := NewAttemptID()
	if err != nil {
		t.Fatal(err)
	}
	twin, err := BuildPod(namespace, "seccomp-twin-"+attempt[:10], image, node, "", profile, false)
	if err != nil {
		t.Fatal(err)
	}
	control, err := BuildPod(namespace, "seccomp-control-"+attempt[:10], image, node, "", profile, true)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	result, err := Run(ctx, pods, namespace, twin, control, 90*time.Second)
	if err != nil {
		t.Fatalf("live verifier runner: %v; result=%+v", err, result)
	}
	if result.Result != Verified {
		t.Fatalf("live twin/control experiment=%s (%s), exits=%v/%v", result.Result, result.Reason, exitOf(result.Twin.ExitCode), exitOf(result.Control.ExitCode))
	}
	if len(twin.UID) == 0 || len(control.UID) == 0 || result.Twin.ContainerID == "" || result.Control.ContainerID == "" {
		t.Fatalf("live Pod runtime identities incomplete: %+v", result)
	}
}

func exitOf(code *int32) string {
	if code == nil {
		return "UNKNOWN"
	}
	return fmt.Sprint(*code)
}
