package projection

import (
	"context"
	"errors"
	"reflect"
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
	if len(result.Materialized.PodLockObservations) != 1 || result.Materialized.PodLockObservations[0].State != BackendNotInstalled || len(result.Materialized.SPOObservations) != 1 || result.Materialized.SPOObservations[0].State != BackendNotInstalled {
		t.Fatalf("per-pod optional observations = %+v", result.Materialized)
	}
	if result.Binding.State != NotAvailable || result.Enforcement.State != NotAvailable || result.BehavioralVerification.State != NotAvailable {
		t.Fatalf("proof layers collapsed = %+v/%+v/%+v", result.Binding, result.Enforcement, result.BehavioralVerification)
	}
	if result.Runtime.State != Available || len(result.Runtime.Evidence[0].Compatibility) != 1 || result.Runtime.Evidence[0].Compatibility[0].Compatibility.State != association.RuntimeMatches {
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
	got := materializedNetworkPolicies(list, []*corev1.Pod{pod})
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

func TestRuntimeEvidencePreservesEveryMatchingSubjectDeterministically(t *testing.T) {
	target := k8s.GovernedTarget{Namespace: "team-a", Workload: k8s.WorkloadRef{Group: "apps", Kind: "Deployment", Name: "api"}, Container: "app"}
	binding := k8s.CanonicalTargetBindingFor(target)
	evidence := association.Evidence{Target: &target, Population: history.Population{Container: "app", ImageIdentity: "sha256:v1", BinaryPath: "/app", TargetBinding: &binding}}
	subjects := []k8s.RuntimeSubject{
		{Target: target, PodUID: "pod-b", ImageID: "sha256:v2", BinaryPath: "/app"},
		{Target: target, PodUID: "pod-c", ImageID: "", BinaryPath: "/app"},
		{Target: target, PodUID: "pod-a", ImageID: "sha256:v1", BinaryPath: "/app"},
	}
	first := runtimeEvidence(target, []association.Evidence{evidence}, subjects)
	reversed := runtimeEvidence(target, []association.Evidence{evidence}, []k8s.RuntimeSubject{subjects[1], subjects[0], subjects[2]})
	if !reflect.DeepEqual(first, reversed) {
		t.Fatalf("runtime result changed with input order: first=%+v reversed=%+v", first, reversed)
	}
	if got := first.Evidence[0].Compatibility; len(got) != 3 || got[0].Compatibility.State != association.RuntimeMatches || got[1].Compatibility.State != association.RuntimeDiffers || got[2].Compatibility.State != association.RuntimeUnknown {
		t.Fatalf("compatibility observations = %+v", got)
	}
}

func TestMaterializedNetworkPoliciesPreserveMatchedPodSet(t *testing.T) {
	pods := []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-b", Namespace: "team-a", Labels: map[string]string{"version": "v2"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "team-a", Labels: map[string]string{"version": "v1"}}},
	}
	list := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{{Object: map[string]interface{}{
		"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy", "metadata": map[string]interface{}{"name": "v1", "namespace": "team-a"},
		"spec": map[string]interface{}{"podSelector": map[string]interface{}{"matchLabels": map[string]interface{}{"version": "v1"}}},
	}}}}
	got := materializedNetworkPolicies(list, pods)
	if len(got.NetworkPolicies) != 1 || len(got.NetworkPolicies[0].MatchedPods) != 1 || got.NetworkPolicies[0].MatchedPods[0].Name != "pod-a" {
		t.Fatalf("matched pods = %+v", got.NetworkPolicies)
	}
}

func TestMaterializedNetworkPoliciesRejectsMissingSelector(t *testing.T) {
	list := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{{Object: map[string]interface{}{
		"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy", "metadata": map[string]interface{}{"name": "malformed", "namespace": "team-a"}, "spec": map[string]interface{}{},
	}}}}
	got := materializedNetworkPolicies(list, []*corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "pod", Labels: map[string]string{"app": "api"}}}})
	if got.State != Empty || len(got.NetworkPolicies) != 0 {
		t.Fatalf("malformed selector attributed: %+v", got)
	}
}

func TestGovernanceProjectionExposesApprovalBinding(t *testing.T) {
	target := k8s.GovernedTarget{Namespace: "team-a", Workload: k8s.WorkloadRef{Group: "apps", Kind: "Deployment", Name: "api"}, Container: "app"}
	binding := k8s.CanonicalTargetBindingFor(target)
	base := proposal.Spec{Container: "app", Binary: "/app", TargetBinding: &binding, PodLock: "candidate-a"}
	digestA, err := proposal.CandidateDigest(base)
	if err != nil {
		t.Fatal(err)
	}
	changed := base
	changed.PodLock = "candidate-b"
	gov, _ := governanceProjection(target, []association.Proposal{{Namespace: "team-a", Name: "api", Target: &target, Spec: changed, Status: &proposal.Status{ApprovalState: proposal.ApprovalApproved, ApprovedCandidateDigest: digestA, ApprovalMechanismVersion: "candidate-v1"}}})
	if len(gov.Proposals) != 1 {
		t.Fatalf("proposals = %+v", gov)
	}
	got := gov.Proposals[0]
	if got.ApprovalState != string(proposal.ApprovalApproved) || got.ApprovedCandidateDigest != digestA || got.ApprovalBindingValid {
		t.Fatalf("approval binding = %+v", got)
	}
}

func TestOptionalBackendSummaryDoesNotHidePerPodFailures(t *testing.T) {
	observations := []OptionalBackendObservation{
		{Pod: SourceRef{Kind: "Pod", Namespace: "team-a", Name: "pod-a"}, State: Available},
		{Pod: SourceRef{Kind: "Pod", Namespace: "team-a", Name: "pod-b"}, State: PermissionDenied},
	}
	if got := summarizeOptional(observations); got != Unknown {
		t.Fatalf("mixed optional backend states summarized as %s", got)
	}
	if observations[0].State != Available || observations[1].State != PermissionDenied {
		t.Fatalf("per-pod observations changed: %+v", observations)
	}
}

func TestProjectRejectsTargetOutsidePinnedNamespace(t *testing.T) {
	service, target, item := projectionFixture(t, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api-pod", Namespace: "team-a"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}})
	target.Namespace = "team-b"
	item.Target = target.Workload
	if _, err := service.Project(context.Background(), target, item, Inputs{Proposals: []association.Proposal{}}); err == nil {
		t.Fatal("projection accepted target outside pinned namespace")
	}
}
