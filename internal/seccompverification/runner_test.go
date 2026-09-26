package seccompverification

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

type memoryPods struct {
	items       map[string]*corev1.Pod
	code        map[string]int32
	image       string
	deleteErr   error
	blockCreate bool
}

func (m *memoryPods) Create(ctx context.Context, pod *corev1.Pod, _ metav1.CreateOptions) (*corev1.Pod, error) {
	if m.blockCreate {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	cp := pod.DeepCopy()
	cp.UID = types.UID("uid-" + cp.Name)
	cp.Status = corev1.PodStatus{Phase: corev1.PodSucceeded, ContainerStatuses: []corev1.ContainerStatus{{Name: ProbeContainerName, ImageID: m.image, ContainerID: "containerd://" + cp.Name, State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: m.code[cp.Name]}}}}}
	m.items[cp.Name] = cp
	return cp, nil
}
func (m *memoryPods) Get(_ context.Context, name string, _ metav1.GetOptions) (*corev1.Pod, error) {
	if p := m.items[name]; p != nil {
		return p.DeepCopy(), nil
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, name)
}
func (m *memoryPods) Delete(_ context.Context, name string, options metav1.DeleteOptions) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	p := m.items[name]
	if p == nil {
		return apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, name)
	}
	if options.Preconditions == nil || options.Preconditions.UID == nil || *options.Preconditions.UID != p.UID {
		return errors.New("UID precondition missing or mismatched")
	}
	delete(m.items, name)
	return nil
}

func runnerPods(t *testing.T, codes map[string]int32) (*memoryPods, *corev1.Pod, *corev1.Pod) {
	t.Helper()
	digest := strings.Repeat("a", 64)
	image := "registry.example/probe@sha256:" + digest
	pods := &memoryPods{items: map[string]*corev1.Pod{}, code: codes, image: "containerd://sha256:" + digest}
	twin, err := BuildPod("verify", "twin-test", image, "node-a", "", "profiles/api.json", false)
	if err != nil {
		t.Fatal(err)
	}
	control, err := BuildPod("verify", "control-test", image, "node-a", "", "profiles/api.json", true)
	if err != nil {
		t.Fatal(err)
	}
	return pods, twin, control
}

func TestRunFixedTwinAndControlAndCleansExactPods(t *testing.T) {
	pods, twin, control := runnerPods(t, map[string]int32{"twin-test": ExitEPERM, "control-test": ExitSuccess})
	result, err := Run(context.Background(), pods, "verify", twin, control, time.Second)
	if err != nil || result.Result != Verified {
		t.Fatalf("Run=%+v err=%v", result, err)
	}
	if len(pods.items) != 0 {
		t.Fatalf("Pods remain after cleanup: %#v", pods.items)
	}
}

func TestRunControlFailureIsUnknownAndCleanupFailureInvalidatesResult(t *testing.T) {
	pods, twin, control := runnerPods(t, map[string]int32{"twin-test": ExitEPERM, "control-test": ExitInconclusive})
	result, err := Run(context.Background(), pods, "verify", twin, control, time.Second)
	if err != nil || result.Result != Unknown {
		t.Fatalf("failed control result=%+v err=%v", result, err)
	}

	pods, twin, control = runnerPods(t, map[string]int32{"twin-test": ExitEPERM, "control-test": ExitSuccess})
	pods.deleteErr = errors.New("delete denied")
	result, err = Run(context.Background(), pods, "verify", twin, control, time.Second)
	if err == nil || result.Result != Unknown || !strings.Contains(result.Reason, "cleanup") {
		t.Fatalf("cleanup failure result=%+v err=%v", result, err)
	}
}

func TestRunTimeoutIsUnknownAndNeverCreatesUnboundedWork(t *testing.T) {
	pods, twin, control := runnerPods(t, map[string]int32{})
	pods.blockCreate = true
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	result, err := Run(ctx, pods, "verify", twin, control, time.Second)
	if err == nil || result.Result != Unknown {
		t.Fatalf("timeout result=%+v err=%v", result, err)
	}
}

func TestRunRejectsInjectedCommand(t *testing.T) {
	pods, twin, control := runnerPods(t, map[string]int32{})
	twin.Spec.Containers[0].Command = []string{"/bin/sh", "-c", "id"}
	_, err := Run(context.Background(), pods, "verify", twin, control, time.Second)
	if err == nil || !strings.Contains(fmt.Sprint(err), "fixed command") {
		t.Fatalf("arbitrary command accepted: %v", err)
	}
}
