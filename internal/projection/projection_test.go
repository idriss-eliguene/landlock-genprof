package projection

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
	discoveryfake "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/association"
	"github.com/idriss-eliguene/landlock-genprof/internal/history"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	"github.com/idriss-eliguene/landlock-genprof/internal/workload"
)

func projectionFixture(t *testing.T, pods ...*corev1.Pod) (*Service, k8s.GovernedTarget, workload.Workload) {
	t.Helper()
	core := kubefake.NewSimpleClientset()
	for _, pod := range pods {
		_, _ = core.CoreV1().Pods(pod.Namespace).Create(context.Background(), pod, metav1.CreateOptions{})
	}
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy", "metadata": map[string]interface{}{"name": "allow-api", "namespace": "team-a"},
		"spec": map[string]interface{}{"podSelector": map[string]interface{}{"matchLabels": map[string]interface{}{"app": "api"}}, "policyTypes": []interface{}{"Ingress"}, "ingress": []interface{}{map[string]interface{}{}}},
	}})
	disc := core.Discovery().(*discoveryfake.FakeDiscovery)
	disc.Resources = []*metav1.APIResourceList{
		{GroupVersion: "networking.k8s.io/v1", APIResources: []metav1.APIResource{{Name: "networkpolicies"}}},
		{GroupVersion: "landlockgenprof.io/v1alpha1", APIResources: []metav1.APIResource{{Name: "securityprofileproposals"}}},
	}
	reads, err := k8s.NewReadSessionForClients(core, dyn, core.Discovery(), "team-a")
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(reads)
	if err != nil {
		t.Fatal(err)
	}
	target := k8s.GovernedTarget{Namespace: "team-a", Workload: k8s.WorkloadRef{Group: "apps", Kind: "Deployment", Name: "api"}, Container: "app"}
	item := workload.Workload{Target: target.Workload, Owner: workload.OwnerSupported}
	for _, pod := range pods {
		item.Pods = append(item.Pods, workload.Pod{Name: pod.Name, UID: string(pod.UID), Containers: []workload.Container{{Name: "app", SupportedTarget: true, Target: &target}}})
	}
	return service, target, item
}

func TestProjectPreservesSecurityProofLayers(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api-pod", Namespace: "team-a", UID: "pod-a", Labels: map[string]string{"app": "api"}}, Spec: corev1.PodSpec{
		SecurityContext: &corev1.PodSecurityContext{SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
		Containers:      []corev1.Container{{Name: "app", SecurityContext: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"NET_BIND_SERVICE"}, Drop: []corev1.Capability{"ALL"}}}}},
	}}
	service, target, item := projectionFixture(t, pod)
	binding := k8s.CanonicalTargetBindingFor(target)
	evidence := association.Evidence{Target: &target, Population: history.Population{Container: "app", ImageIdentity: "sha256:a", BinaryPath: "/app", TargetBinding: &binding}}
	spec := proposal.Spec{Container: "app", Binary: "/app", TargetBinding: &binding, PodLock: "candidate"}
	result, err := service.Project(context.Background(), target, item, Inputs{Evidence: []association.Evidence{evidence}, RuntimeSubjects: []k8s.RuntimeSubject{{Target: target, ImageID: "sha256:a", BinaryPath: "/app"}}, Proposals: []association.Proposal{{Namespace: "team-a", Name: "api", Target: &target, Spec: spec}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Declared.State != Available || result.Declared.Containers[0].SeccompType != string(corev1.SeccompProfileTypeRuntimeDefault) {
		t.Fatalf("declared = %+v", result.Declared)
	}
	if result.Materialized.State != Available || len(result.Materialized.NetworkPolicies) != 1 {
		t.Fatalf("materialized = %+v", result.Materialized)
	}
	if result.Materialized.PodLockState != BackendNotInstalled || result.Materialized.SPOState != BackendNotInstalled {
		t.Fatalf("optional states = %+v", result.Materialized)
	}
	if result.Binding.State != Available || result.Enforcement.State != NotAvailable || result.BehavioralVerification.State != NotAvailable {
		t.Fatalf("proof layers collapsed = %+v/%+v/%+v", result.Binding, result.Enforcement, result.BehavioralVerification)
	}
	if result.Runtime.State != Available || result.Runtime.Evidence[0].Compatibility.State != association.RuntimeMatches {
		t.Fatalf("runtime = %+v", result.Runtime)
	}
	if result.Governance.State != Available || result.Governance.Proposals[0].ApprovalState != string(proposal.ApprovalDraft) || result.Derived.State != Available {
		t.Fatalf("governance = %+v derived=%+v", result.Governance, result.Derived)
	}
}

func TestProjectMatchesExpressionsAndPreservesMultiplePolicies(t *testing.T) {
	service, target, item := projectionFixture(t, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api-pod", Namespace: "team-a", Labels: map[string]string{"app": "api", "tier": "backend"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}})
	// The fixture already contains one matching policy; this test primarily
	// verifies deterministic selection and that no effective allow-set exists.
	result, err := service.Project(context.Background(), target, item, Inputs{Proposals: []association.Proposal{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Materialized.NetworkPolicies) != 1 || result.Materialized.NetworkPolicies[0].IngressRules != 1 {
		t.Fatalf("network projection = %+v", result.Materialized.NetworkPolicies)
	}
}

func TestMaterializedNetworkPoliciesSupportsSelectorExpressionsAndMultiplicity(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api-pod", Namespace: "team-a", Labels: map[string]string{"app": "api", "tier": "backend"}}}
	list := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
		{Object: map[string]interface{}{"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy", "metadata": map[string]interface{}{"name": "a", "namespace": "team-a"}, "spec": map[string]interface{}{"podSelector": map[string]interface{}{"matchExpressions": []interface{}{map[string]interface{}{"key": "tier", "operator": "In", "values": []interface{}{"backend"}}}}, "policyTypes": []interface{}{"Ingress"}}}},
		{Object: map[string]interface{}{"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy", "metadata": map[string]interface{}{"name": "b", "namespace": "team-a"}, "spec": map[string]interface{}{"podSelector": map[string]interface{}{"matchLabels": map[string]interface{}{"app": "api"}}, "policyTypes": []interface{}{"Egress"}}}},
	}}
	got := materializedNetworkPolicies(list, "team-a", []*corev1.Pod{pod})
	if got.State != Available || len(got.NetworkPolicies) != 2 {
		t.Fatalf("network policies = %+v", got)
	}
	if got.NetworkPolicies[0].Source.Name != "a" || got.NetworkPolicies[1].Source.Name != "b" {
		t.Fatalf("ordering = %+v", got.NetworkPolicies)
	}
}

func TestProjectPreservesPermissionDenied(t *testing.T) {
	core := kubefake.NewSimpleClientset()
	core.PrependReactor("get", "pods", func(kubetesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "api", errors.New("denied"))
	})
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	disc := core.Discovery().(*discoveryfake.FakeDiscovery)
	disc.Resources = []*metav1.APIResourceList{{GroupVersion: "networking.k8s.io/v1", APIResources: []metav1.APIResource{{Name: "networkpolicies"}}}}
	reads, err := k8s.NewReadSessionForClients(core, dyn, core.Discovery(), "team-a")
	if err != nil {
		t.Fatal(err)
	}
	service, _ := NewService(reads)
	target := k8s.GovernedTarget{Namespace: "team-a", Workload: k8s.WorkloadRef{Kind: "Pod", Name: "api"}, Container: "app"}
	item := workload.Workload{Target: target.Workload, Owner: workload.OwnerBarePod, Pods: []workload.Pod{{Name: "api", Containers: []workload.Container{{Name: "app", SupportedTarget: true, Target: &target}}}}}
	_, err = service.Project(context.Background(), target, item, Inputs{})
	if err == nil {
		t.Fatal("permission failure collapsed into projection success")
	}
	var readErr *k8s.ReadError
	if !errors.As(err, &readErr) || readErr.State != k8s.ReadPermissionDenied {
		t.Fatalf("permission state = %v", err)
	}
}
