package workload

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
)

func pod(name, uid string, owners []metav1.OwnerReference, containers []corev1.Container) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "team-a", UID: types.UID(uid), OwnerReferences: owners}, Spec: corev1.PodSpec{Containers: containers}}
}

func controller(kind, name string) metav1.OwnerReference {
	trueValue := true
	return metav1.OwnerReference{APIVersion: "apps/v1", Kind: kind, Name: name, UID: types.UID(name + "-uid"), Controller: &trueValue}
}

func replicaSet(name, deployment string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "ReplicaSet", "metadata": map[string]interface{}{
			"name": name, "namespace": "team-a", "uid": name + "-uid", "ownerReferences": []interface{}{map[string]interface{}{
				"apiVersion": "apps/v1", "kind": "Deployment", "name": deployment, "uid": deployment + "-uid", "controller": true,
			}},
		},
	}}
}

func TestDiscoverGroupsReplicasAndEnumeratesContainerCategories(t *testing.T) {
	podA := pod("api-a", "uid-a", []metav1.OwnerReference{controller("ReplicaSet", "api-rs-a")}, []corev1.Container{{Name: "app"}, {Name: "proxy"}})
	podA.Spec.InitContainers = []corev1.Container{{Name: "init"}, {Name: "sidecar", RestartPolicy: ptr(corev1.ContainerRestartPolicyAlways)}}
	podA.Spec.EphemeralContainers = []corev1.EphemeralContainer{{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "debug"}}}
	podA.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "app", ImageID: "sha256:a"}}
	podA.Status.InitContainerStatuses = []corev1.ContainerStatus{{Name: "sidecar", ImageID: "sha256:s"}}
	podB := pod("api-b", "uid-b", []metav1.OwnerReference{controller("ReplicaSet", "api-rs-b")}, []corev1.Container{{Name: "app"}})
	podB.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "app", ImageID: "sha256:b"}}
	core := kubefake.NewSimpleClientset(podA, podB)
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), replicaSet("api-rs-a", "api"), replicaSet("api-rs-b", "api"))
	disc := core.Discovery().(*fake.FakeDiscovery)
	disc.Resources = []*metav1.APIResourceList{{GroupVersion: "apps/v1", APIResources: []metav1.APIResource{{Name: "replicasets"}}}}
	reads, err := k8s.NewReadSessionForClients(core, dyn, core.Discovery(), "team-a")
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(reads)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Workloads) != 1 || result.Workloads[0].Target.Name != "api" || len(result.Workloads[0].Pods) != 2 {
		t.Fatalf("result = %+v", result)
	}
	containers := result.Workloads[0].Pods[0].Containers
	if len(containers) != 5 {
		t.Fatalf("containers = %+v", containers)
	}
	for _, c := range containers {
		switch c.Name {
		case "app", "proxy":
			if !c.SupportedTarget || c.Target == nil {
				t.Fatalf("regular container not supported: %+v", c)
			}
		case "init":
			if c.Category != ContainerInit || c.SupportedTarget {
				t.Fatalf("init classification = %+v", c)
			}
		case "sidecar":
			if c.Category != ContainerNativeSide || c.SupportedTarget {
				t.Fatalf("sidecar classification = %+v", c)
			}
		case "debug":
			if c.Category != ContainerEphemeral || c.SupportedTarget {
				t.Fatalf("ephemeral classification = %+v", c)
			}
		}
	}
	if containers[0].RuntimeState != RuntimeAvailable || containers[1].RuntimeState != RuntimeStatusUnavailable {
		t.Fatalf("runtime states = %+v", containers)
	}
}

func TestDiscoverDoesNotDowngradeUnsupportedOrMissingOwnersToBarePods(t *testing.T) {
	jobPod := pod("job-pod", "job-uid", []metav1.OwnerReference{controller("Job", "job")}, []corev1.Container{{Name: "app"}})
	orphan := pod("orphan", "orphan-uid", []metav1.OwnerReference{controller("ReplicaSet", "gone")}, []corev1.Container{{Name: "app"}})
	core := kubefake.NewSimpleClientset(jobPod, orphan)
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	disc := core.Discovery().(*fake.FakeDiscovery)
	disc.Resources = []*metav1.APIResourceList{{GroupVersion: "apps/v1", APIResources: []metav1.APIResource{{Name: "replicasets"}}}}
	reads, err := k8s.NewReadSessionForClients(core, dyn, core.Discovery(), "team-a")
	if err != nil {
		t.Fatal(err)
	}
	service, _ := NewService(reads)
	result, err := service.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Workloads) != 2 {
		t.Fatalf("workloads = %+v", result.Workloads)
	}
	for _, item := range result.Workloads {
		if item.Owner == OwnerBarePod {
			t.Fatalf("unsupported owner downgraded: %+v", item)
		}
		if item.Pods[0].Containers[0].SupportedTarget {
			t.Fatalf("unsupported owner target: %+v", item)
		}
	}
}

func TestDiscoverSupportsDirectWorkloadOwnersAndBarePods(t *testing.T) {
	deploymentPod := pod("deployment-pod", "d-uid", []metav1.OwnerReference{controller("Deployment", "web")}, []corev1.Container{{Name: "app"}})
	statefulPod := pod("stateful-pod", "s-uid", []metav1.OwnerReference{controller("StatefulSet", "db")}, []corev1.Container{{Name: "db"}})
	daemonPod := pod("daemon-pod", "ds-uid", []metav1.OwnerReference{controller("DaemonSet", "agent")}, []corev1.Container{{Name: "agent"}})
	barePod := pod("bare-pod", "bare-uid", nil, []corev1.Container{{Name: "app"}})
	core := kubefake.NewSimpleClientset(deploymentPod, statefulPod, daemonPod, barePod)
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(),
		workloadObject("Deployment", "web"), workloadObject("StatefulSet", "db"), workloadObject("DaemonSet", "agent"))
	disc := core.Discovery().(*fake.FakeDiscovery)
	disc.Resources = []*metav1.APIResourceList{{GroupVersion: "apps/v1", APIResources: []metav1.APIResource{{Name: "deployments"}, {Name: "statefulsets"}, {Name: "daemonsets"}}}}
	reads, err := k8s.NewReadSessionForClients(core, dyn, core.Discovery(), "team-a")
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(reads)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Workloads) != 4 {
		t.Fatalf("workload count = %d, result = %+v", len(result.Workloads), result)
	}
	for _, item := range result.Workloads {
		if item.Owner != OwnerSupported && item.Owner != OwnerBarePod {
			t.Fatalf("unexpected owner state: %+v", item)
		}
		if len(item.Pods) != 1 || !item.Pods[0].Containers[0].SupportedTarget {
			t.Fatalf("unsupported target: %+v", item)
		}
	}
}

func workloadObject(kind, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": kind, "metadata": map[string]interface{}{
			"name": name, "namespace": "team-a", "uid": name + "-uid",
		},
	}}
}

func TestDiscoverRejectsOwnerGroupMismatch(t *testing.T) {
	tests := []struct {
		name       string
		kind       string
		apiVersion string
	}{
		{name: "deployment", kind: "Deployment", apiVersion: "evil.example/v1"},
		{name: "replicaset", kind: "ReplicaSet", apiVersion: "evil.example/v1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := pod(test.name, "uid-"+test.name, []metav1.OwnerReference{{APIVersion: test.apiVersion, Kind: test.kind, Name: "api", UID: "api-uid", Controller: ptr(true)}}, []corev1.Container{{Name: "app"}})
			service := testBareService(t, p)
			result, err := service.Discover(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			item := result.Workloads[0]
			if item.Owner != OwnerUnsupported || item.Owner == OwnerSupported || item.Pods[0].Containers[0].SupportedTarget {
				t.Fatalf("foreign group became supported: %+v", item)
			}
		})
	}
}

func TestDiscoverRejectsOwnerUIDMismatch(t *testing.T) {
	p := pod("api", "pod-uid", []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: "api", UID: "old-uid", Controller: ptr(true)}}, []corev1.Container{{Name: "app"}})
	core := kubefake.NewSimpleClientset(p)
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), workloadObject("Deployment", "api"))
	disc := core.Discovery().(*fake.FakeDiscovery)
	disc.Resources = []*metav1.APIResourceList{{GroupVersion: "apps/v1", APIResources: []metav1.APIResource{{Name: "deployments"}}}}
	reads, err := k8s.NewReadSessionForClients(core, dyn, core.Discovery(), "team-a")
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(reads)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Workloads[0].Owner != OwnerUnresolved || result.Workloads[0].Pods[0].Containers[0].SupportedTarget {
		t.Fatalf("UID mismatch became supported: %+v", result.Workloads[0])
	}
}

func TestDiscoverRetainsStatusMismatchAndUnknownImage(t *testing.T) {
	p := pod("api", "uid", nil, []corev1.Container{{Name: "app"}})
	p.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "stale"}, {Name: "app"}}
	service := testBareService(t, p)
	result, err := service.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	container := result.Workloads[0].Pods[0].Containers[0]
	if container.RuntimeState != RuntimeImageUnknown || container.Runtime == nil || container.Runtime.ImageID != "" {
		t.Fatalf("unknown image state = %+v", container)
	}
	if len(result.Workloads[0].Pods[0].UnmatchedRuntimeStatus) != 1 || result.Workloads[0].Pods[0].UnmatchedRuntimeStatus[0] != "stale" {
		t.Fatalf("unmatched statuses = %+v", result.Workloads[0].Pods[0].UnmatchedRuntimeStatus)
	}
}

func testBareService(t *testing.T, pods ...*corev1.Pod) *Service {
	t.Helper()
	objects := make([]runtime.Object, len(pods))
	for i, item := range pods {
		objects[i] = item
	}
	core := kubefake.NewSimpleClientset(objects...)
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	reads, err := k8s.NewReadSessionForClients(core, dyn, core.Discovery(), "team-a")
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(reads)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestDiscoverPreservesForbiddenOwnerRead(t *testing.T) {
	p := pod("api", "uid", []metav1.OwnerReference{controller("ReplicaSet", "rs")}, []corev1.Container{{Name: "app"}})
	core := kubefake.NewSimpleClientset(p)
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	dyn.PrependReactor("get", "replicasets", func(kubetesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "replicasets"}, "rs", errors.New("denied"))
	})
	disc := core.Discovery().(*fake.FakeDiscovery)
	disc.Resources = []*metav1.APIResourceList{{GroupVersion: "apps/v1", APIResources: []metav1.APIResource{{Name: "replicasets"}}}}
	reads, err := k8s.NewReadSessionForClients(core, dyn, core.Discovery(), "team-a")
	if err != nil {
		t.Fatal(err)
	}
	service, _ := NewService(reads)
	_, err = service.Discover(context.Background())
	var readErr *k8s.ReadError
	if !errors.As(err, &readErr) || readErr.State != k8s.ReadPermissionDenied {
		t.Fatalf("error = %v", err)
	}
}

func TestDiscoverEmptyAndCancellationAreDistinct(t *testing.T) {
	core := kubefake.NewSimpleClientset()
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	disc := core.Discovery().(*fake.FakeDiscovery)
	reads, err := k8s.NewReadSessionForClients(core, dyn, disc, "team-a")
	if err != nil {
		t.Fatal(err)
	}
	service, _ := NewService(reads)
	result, err := service.Discover(context.Background())
	if err != nil || result.State != StateEmpty {
		t.Fatalf("empty result = %+v, %v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Discover(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
}

func ptr[T any](v T) *T { return &v }
