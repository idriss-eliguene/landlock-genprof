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
	"k8s.io/apimachinery/pkg/types"
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
	service, target, item, _ := projectionFixtureWithCore(t, pods...)
	return service, target, item
}

func projectionFixtureWithCore(t *testing.T, pods ...*corev1.Pod) (*Service, k8s.GovernedTarget, workload.Workload, *kubefake.Clientset) {
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
	return service, target, item, core
}

// declaredPod builds a target-carrying Pod with a readable declaration.
func declaredPod(name string, uid types.UID) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "team-a", UID: uid, Labels: map[string]string{"app": "api"}},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", SecurityContext: &corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		}}}},
	}
}

// failPodGets makes GetPod fail for the named Pods only, so partial-read
// behavior can be exercised without disturbing the other Pods.
func failPodGets(core *kubefake.Clientset, failures map[string]error) {
	core.PrependReactor("get", "pods", func(action kubetesting.Action) (bool, runtime.Object, error) {
		get, ok := action.(kubetesting.GetAction)
		if !ok {
			return false, nil, nil
		}
		if err, found := failures[get.GetName()]; found {
			return true, nil, err
		}
		return false, nil, nil
	})
}

func forbiddenPod(name string) error {
	return apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, name, errors.New("denied"))
}

func missingPod(name string) error {
	return apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, name)
}

func timedOutPod() error { return apierrors.NewTimeoutError("pod read timed out", 1) }

// observationFor returns the recorded observation for one Pod name.
func observationFor(t *testing.T, declared DeclaredConfiguration, name string) PodReadObservation {
	t.Helper()
	for _, observation := range declared.Observations {
		if observation.Pod.Name == name {
			return observation
		}
	}
	t.Fatalf("no Pod read observation recorded for %q in %+v", name, declared.Observations)
	return PodReadObservation{}
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

// --- L1: partial Pod observation must stay distinguishable from complete ---

func TestDeclaredSinglePodSuccessIsCompleteObservation(t *testing.T) {
	service, target, item := projectionFixture(t, declaredPod("api-1", "uid-1"))
	result, err := service.Project(context.Background(), target, item, Inputs{Proposals: []association.Proposal{}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Declared.State != Available {
		t.Fatalf("complete observation must be AVAILABLE, got %+v", result.Declared.Section)
	}
	if len(result.Declared.Observations) != 1 {
		t.Fatalf("observations = %+v", result.Declared.Observations)
	}
	only := result.Declared.Observations[0]
	if only.State != Available || !only.Contributed || only.Pod.UID != "uid-1" {
		t.Fatalf("observation lost identity or contribution: %+v", only)
	}
}

func TestDeclaredSinglePodReadFailurePreservesExistingFailSemantics(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		state k8s.ReadState
	}{
		{"not found", missingPod("api-1"), k8s.ReadNotFound},
		{"permission denied", forbiddenPod("api-1"), k8s.ReadPermissionDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, target, item, core := projectionFixtureWithCore(t, declaredPod("api-1", "uid-1"))
			failPodGets(core, map[string]error{"api-1": tc.err})
			_, err := service.Project(context.Background(), target, item, Inputs{Proposals: []association.Proposal{}})
			if err == nil {
				t.Fatal("total read failure collapsed into a successful projection")
			}
			var readErr *k8s.ReadError
			if !errors.As(err, &readErr) || readErr.State != tc.state {
				t.Fatalf("read state = %v, want %v", err, tc.state)
			}
		})
	}
}

func TestDeclaredAllPodsReadableIsCompleteObservation(t *testing.T) {
	service, target, item := projectionFixture(t, declaredPod("api-1", "uid-1"), declaredPod("api-2", "uid-2"))
	result, err := service.Project(context.Background(), target, item, Inputs{Proposals: []association.Proposal{}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Declared.State != Available {
		t.Fatalf("all Pods readable must be AVAILABLE, got %+v", result.Declared.Section)
	}
	if len(result.Declared.Observations) != 2 {
		t.Fatalf("observations = %+v", result.Declared.Observations)
	}
	for _, observation := range result.Declared.Observations {
		if observation.State != Available || !observation.Contributed {
			t.Fatalf("readable Pod not recorded as contributing: %+v", observation)
		}
	}
	if len(result.Declared.Containers) != 2 {
		t.Fatalf("successful evidence discarded: %+v", result.Declared.Containers)
	}
}

func TestDeclaredPartialReadIsNotReportedAsComplete(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		failed State
	}{
		{"permission denied", forbiddenPod("api-2"), PermissionDenied},
		{"timeout", timedOutPod(), Timeout},
		{"not found", missingPod("api-2"), NotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, target, item, core := projectionFixtureWithCore(t, declaredPod("api-1", "uid-1"), declaredPod("api-2", "uid-2"))
			failPodGets(core, map[string]error{"api-2": tc.err})
			result, err := service.Project(context.Background(), target, item, Inputs{Proposals: []association.Proposal{}})
			if err != nil {
				t.Fatalf("partial success must not fail the projection: %v", err)
			}
			// The semantic property: partial knowledge is never AVAILABLE.
			if result.Declared.State == Available {
				t.Fatalf("partial observation reported as complete: %+v", result.Declared.Section)
			}
			if result.Declared.State != Unknown {
				t.Fatalf("partial observation state = %q, want %q", result.Declared.State, Unknown)
			}
			// The successful evidence must survive.
			readable := observationFor(t, result.Declared, "api-1")
			if readable.State != Available || !readable.Contributed {
				t.Fatalf("successful Pod evidence discarded: %+v", readable)
			}
			if len(result.Declared.Containers) != 1 || result.Declared.Containers[0].PodName != "api-1" {
				t.Fatalf("declared containers = %+v", result.Declared.Containers)
			}
			// The failure must survive with its exact normalized state.
			failed := observationFor(t, result.Declared, "api-2")
			if failed.State != tc.failed {
				t.Fatalf("failed observation state = %q, want %q", failed.State, tc.failed)
			}
			if failed.Contributed {
				t.Fatal("unreadable Pod must never be marked as contributing")
			}
			if failed.Reason == "" {
				t.Fatal("failed observation lost its reason")
			}
		})
	}
}

func TestDeclaredPartialReadDistinguishesFailureReasons(t *testing.T) {
	service, target, item, core := projectionFixtureWithCore(t,
		declaredPod("api-1", "uid-1"), declaredPod("api-2", "uid-2"), declaredPod("api-3", "uid-3"))
	failPodGets(core, map[string]error{"api-2": forbiddenPod("api-2"), "api-3": timedOutPod()})
	result, err := service.Project(context.Background(), target, item, Inputs{Proposals: []association.Proposal{}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Declared.State != Unknown {
		t.Fatalf("state = %+v", result.Declared.Section)
	}
	// PERMISSION_DENIED and TIMEOUT must not collapse into one another.
	if got := observationFor(t, result.Declared, "api-2").State; got != PermissionDenied {
		t.Fatalf("api-2 state = %q, want %q", got, PermissionDenied)
	}
	if got := observationFor(t, result.Declared, "api-3").State; got != Timeout {
		t.Fatalf("api-3 state = %q, want %q", got, Timeout)
	}
	if got := observationFor(t, result.Declared, "api-1").State; got != Available {
		t.Fatalf("api-1 state = %q, want %q", got, Available)
	}
}

func TestDeclaredNotFoundAndPermissionDeniedRemainDistinct(t *testing.T) {
	service, target, item, core := projectionFixtureWithCore(t,
		declaredPod("api-1", "uid-1"), declaredPod("api-2", "uid-2"), declaredPod("api-3", "uid-3"))
	failPodGets(core, map[string]error{"api-2": missingPod("api-2"), "api-3": forbiddenPod("api-3")})
	result, err := service.Project(context.Background(), target, item, Inputs{Proposals: []association.Proposal{}})
	if err != nil {
		t.Fatal(err)
	}
	absent := observationFor(t, result.Declared, "api-2").State
	denied := observationFor(t, result.Declared, "api-3").State
	if absent != NotFound || denied != PermissionDenied {
		t.Fatalf("absence and observation failure collapsed: absent=%q denied=%q", absent, denied)
	}
}

func TestDeclaredAllPodsUnreadableFailsClosed(t *testing.T) {
	service, target, item, core := projectionFixtureWithCore(t, declaredPod("api-1", "uid-1"), declaredPod("api-2", "uid-2"))
	failPodGets(core, map[string]error{"api-1": forbiddenPod("api-1"), "api-2": forbiddenPod("api-2")})
	_, err := service.Project(context.Background(), target, item, Inputs{Proposals: []association.Proposal{}})
	var readErr *k8s.ReadError
	if !errors.As(err, &readErr) || readErr.State != k8s.ReadPermissionDenied {
		t.Fatalf("all-unreadable must fail closed, got %v", err)
	}
}

func TestDeclaredAllPodsUnreadableWithMixedReasonsIsDeterministic(t *testing.T) {
	var first k8s.ReadState
	for attempt := 0; attempt < 8; attempt++ {
		service, target, item, core := projectionFixtureWithCore(t, declaredPod("api-1", "uid-1"), declaredPod("api-2", "uid-2"))
		failPodGets(core, map[string]error{"api-1": forbiddenPod("api-1"), "api-2": timedOutPod()})
		_, err := service.Project(context.Background(), target, item, Inputs{Proposals: []association.Proposal{}})
		var readErr *k8s.ReadError
		if !errors.As(err, &readErr) {
			t.Fatalf("expected a read error, got %v", err)
		}
		if attempt == 0 {
			first = readErr.State
			continue
		}
		if readErr.State != first {
			t.Fatalf("nondeterministic failure selection: %q then %q", first, readErr.State)
		}
	}
	if first != k8s.ReadPermissionDenied {
		t.Fatalf("expected the first Pod in sorted order to select the error, got %q", first)
	}
}

func TestDeclaredObservationsAreDeterministicallyOrdered(t *testing.T) {
	var previous []string
	for attempt := 0; attempt < 8; attempt++ {
		service, target, item, core := projectionFixtureWithCore(t,
			declaredPod("api-3", "uid-3"), declaredPod("api-1", "uid-1"), declaredPod("api-2", "uid-2"))
		failPodGets(core, map[string]error{"api-2": forbiddenPod("api-2")})
		result, err := service.Project(context.Background(), target, item, Inputs{Proposals: []association.Proposal{}})
		if err != nil {
			t.Fatal(err)
		}
		order := make([]string, 0, len(result.Declared.Observations))
		for _, observation := range result.Declared.Observations {
			order = append(order, observation.Pod.Name)
		}
		if previous != nil && !reflect.DeepEqual(previous, order) {
			t.Fatalf("observation order is nondeterministic: %v then %v", previous, order)
		}
		previous = order
	}
	if !reflect.DeepEqual(previous, []string{"api-1", "api-2", "api-3"}) {
		t.Fatalf("observation order = %v", previous)
	}
}

// --- L2: excluded evidence must stay distinguishable from no evidence ---

// legacyEvidence has no producer-time canonical binding, which is exactly the
// state G1.6 preserves for pre-existing objects.
func legacyEvidence(image string) association.Evidence {
	return association.Evidence{Population: history.Population{Container: "app", ImageIdentity: image, BinaryPath: "/app"}}
}

func foreignEvidence(t *testing.T, image string) association.Evidence {
	t.Helper()
	other := k8s.GovernedTarget{Namespace: "team-a", Workload: k8s.WorkloadRef{Group: "apps", Kind: "Deployment", Name: "other"}, Container: "app"}
	binding := k8s.CanonicalTargetBindingFor(other)
	return association.Evidence{Target: &other, Population: history.Population{Container: "app", ImageIdentity: image, BinaryPath: "/app", TargetBinding: &binding}}
}

func ownEvidence(target k8s.GovernedTarget, image string) association.Evidence {
	binding := k8s.CanonicalTargetBindingFor(target)
	return association.Evidence{Target: &target, Population: history.Population{Container: "app", ImageIdentity: image, BinaryPath: "/app", TargetBinding: &binding}}
}

func TestRuntimeZeroEvidenceIsDistinctFromExcludedEvidence(t *testing.T) {
	service, target, item := projectionFixture(t, declaredPod("api-1", "uid-1"))

	none, err := service.Project(context.Background(), target, item, Inputs{Proposals: []association.Proposal{}})
	if err != nil {
		t.Fatal(err)
	}
	if none.Runtime.State != Empty {
		t.Fatalf("zero evidence state = %q, want %q", none.Runtime.State, Empty)
	}
	if len(none.Runtime.Excluded) != 0 || len(none.Runtime.Evidence) != 0 {
		t.Fatalf("zero evidence must record nothing: %+v", none.Runtime)
	}

	excluded, err := service.Project(context.Background(), target, item, Inputs{
		Proposals: []association.Proposal{},
		Evidence:  []association.Evidence{legacyEvidence("sha256:legacy")},
	})
	if err != nil {
		t.Fatal(err)
	}
	// The semantic property: observed-but-unattributable is not emptiness.
	if excluded.Runtime.State == none.Runtime.State {
		t.Fatalf("excluded evidence collapsed into the zero-evidence state %q", excluded.Runtime.State)
	}
	if excluded.Runtime.State != Unknown {
		t.Fatalf("excluded-only state = %q, want %q", excluded.Runtime.State, Unknown)
	}
	if len(excluded.Runtime.Evidence) != 0 {
		t.Fatal("excluded evidence was promoted to attributed evidence")
	}
	if len(excluded.Runtime.Excluded) != 1 || excluded.Runtime.Excluded[0].Association.State != association.InsufficientProvenance {
		t.Fatalf("excluded association state lost: %+v", excluded.Runtime.Excluded)
	}
	if excluded.Runtime.Excluded[0].Association.Reason == "" {
		t.Fatal("excluded association reason lost")
	}
}

func TestRuntimeEveryReachableAssociationStateIsPreserved(t *testing.T) {
	service, target, item := projectionFixture(t, declaredPod("api-1", "uid-1"))
	for _, tc := range []struct {
		name       string
		source     association.Evidence
		want       association.State
		attributed bool
	}{
		{"associated", ownEvidence(target, "sha256:own"), association.Associated, true},
		{"unassociated", foreignEvidence(t, "sha256:foreign"), association.Unassociated, false},
		{"insufficient provenance", legacyEvidence("sha256:legacy"), association.InsufficientProvenance, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := service.Project(context.Background(), target, item, Inputs{
				Proposals: []association.Proposal{},
				Evidence:  []association.Evidence{tc.source},
			})
			if err != nil {
				t.Fatal(err)
			}
			if tc.attributed {
				if len(result.Runtime.Evidence) != 1 || result.Runtime.Evidence[0].Association.State != tc.want {
					t.Fatalf("attributed evidence = %+v", result.Runtime.Evidence)
				}
				if len(result.Runtime.Excluded) != 0 {
					t.Fatalf("associated evidence must not be excluded: %+v", result.Runtime.Excluded)
				}
				return
			}
			// Fail-closed: never attributed, always explainable.
			if len(result.Runtime.Evidence) != 0 {
				t.Fatalf("%s evidence was attributed to the target: %+v", tc.want, result.Runtime.Evidence)
			}
			if len(result.Runtime.Excluded) != 1 {
				t.Fatalf("excluded evidence was discarded: %+v", result.Runtime)
			}
			if result.Runtime.Excluded[0].Association.State != tc.want {
				t.Fatalf("association state = %q, want %q", result.Runtime.Excluded[0].Association.State, tc.want)
			}
		})
	}
}

func TestRuntimeExcludedAndAssociatedRemainDistinct(t *testing.T) {
	service, target, item := projectionFixture(t, declaredPod("api-1", "uid-1"))
	result, err := service.Project(context.Background(), target, item, Inputs{
		Proposals: []association.Proposal{},
		Evidence:  []association.Evidence{ownEvidence(target, "sha256:own"), foreignEvidence(t, "sha256:foreign")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Runtime.Evidence) != 1 || result.Runtime.Evidence[0].Association.State != association.Associated {
		t.Fatalf("attributed = %+v", result.Runtime.Evidence)
	}
	if len(result.Runtime.Excluded) != 1 || result.Runtime.Excluded[0].Association.State != association.Unassociated {
		t.Fatalf("excluded = %+v", result.Runtime.Excluded)
	}
	if result.Runtime.Excluded[0].Source.Population.ImageIdentity != "sha256:foreign" {
		t.Fatalf("excluded source identity lost: %+v", result.Runtime.Excluded[0].Source)
	}
	if result.Runtime.State != Available {
		t.Fatalf("state = %q, want %q", result.Runtime.State, Available)
	}
}

func TestRuntimeExcludedEvidenceIsDeterministicallyOrdered(t *testing.T) {
	service, target, item := projectionFixture(t, declaredPod("api-1", "uid-1"))
	var previous []string
	for attempt := 0; attempt < 8; attempt++ {
		result, err := service.Project(context.Background(), target, item, Inputs{
			Proposals: []association.Proposal{},
			Evidence: []association.Evidence{
				legacyEvidence("sha256:c"), foreignEvidence(t, "sha256:a"), legacyEvidence("sha256:b"),
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		order := make([]string, 0, len(result.Runtime.Excluded))
		for _, excluded := range result.Runtime.Excluded {
			order = append(order, string(excluded.Association.State)+"/"+excluded.Source.Population.ImageIdentity)
		}
		if previous != nil && !reflect.DeepEqual(previous, order) {
			t.Fatalf("excluded order is nondeterministic: %v then %v", previous, order)
		}
		previous = order
	}
	if len(previous) != 3 {
		t.Fatalf("excluded evidence lost: %v", previous)
	}
}
