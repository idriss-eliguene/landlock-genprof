//go:build envtest

package authz

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/authn"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var authorizationEnv *envtest.Environment
var authorizationConfig *rest.Config

func TestMain(m *testing.M) {
	authorizationEnv = &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{Paths: []string{
			"../../deploy/crd-observation.yaml",
			"../../deploy/crd-observationcontributionreceipt.yaml",
			"../../deploy/crd-traininghistory.yaml",
			"../../deploy/crd-securityprofileproposal.yaml",
			"../../deploy/crd-applyattempt.yaml",
			"../../deploy/crd-rollbackattempt.yaml",
		}, ErrorIfPathMissing: true},
		ControlPlane: envtest.ControlPlane{APIServer: &envtest.APIServer{Args: []string{"--authorization-mode=RBAC"}}},
	}
	var err error
	authorizationConfig, err = authorizationEnv.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "authz envtest start: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = authorizationEnv.Stop()
	os.Exit(code)
}

func TestNamespaceLocalRoleBindingsIsolateImpersonatedUsers(t *testing.T) {
	ctx := context.Background()
	admin, err := kubernetes.NewForConfig(authorizationConfig)
	if err != nil {
		t.Fatal(err)
	}
	for _, namespace := range []string{"team-a", "team-b"} {
		if _, err := admin.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.RbacV1().Roles(namespace).Create(ctx, &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "viewer", Namespace: namespace}, Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"}},
			{APIGroups: []string{"landlockgenprof.io"}, Resources: []string{"observations", "observationcontributionreceipts", "traininghistories", "securityprofileproposals", "applyattempts", "rollbackattempts"}, Verbs: []string{"get", "list"}},
		}}, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	for _, binding := range []struct{ namespace, user string }{{"team-a", "alice"}, {"team-b", "bob"}} {
		if _, err := admin.RbacV1().RoleBindings(binding.namespace).Create(ctx, &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "viewer-" + binding.user, Namespace: binding.namespace}, Subjects: []rbacv1.Subject{{Kind: "User", Name: binding.user}}, RoleRef: rbacv1.RoleRef{Kind: "Role", Name: "viewer"}}, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.RbacV1().Roles(binding.namespace).Create(ctx, &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "governance", Namespace: binding.namespace}, Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{"landlockgenprof.io"}, Resources: []string{"securityprofileproposals"}, Verbs: []string{"get"}},
			{APIGroups: []string{"landlockgenprof.io"}, Resources: []string{"securityprofileproposals/status"}, Verbs: []string{"get", "update", "patch"}},
			{APIGroups: []string{"landlockgenprof.io"}, Resources: []string{"applyattempts"}, Verbs: []string{"get", "create"}},
			{APIGroups: []string{"landlockgenprof.io"}, Resources: []string{"applyattempts/status"}, Verbs: []string{"get", "update", "patch"}},
			{APIGroups: []string{"landlockgenprof.io"}, Resources: []string{"rollbackattempts"}, Verbs: []string{"get", "create"}},
			{APIGroups: []string{"landlockgenprof.io"}, Resources: []string{"rollbackattempts/status"}, Verbs: []string{"get", "update", "patch"}},
		}}, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.RbacV1().RoleBindings(binding.namespace).Create(ctx, &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "governance-" + binding.user, Namespace: binding.namespace}, Subjects: []rbacv1.Subject{{Kind: "User", Name: binding.user}}, RoleRef: rbacv1.RoleRef{Kind: "Role", Name: "governance"}}, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := admin.RbacV1().Roles("team-a").Create(ctx, &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "operator", Namespace: "team-a"}, Rules: []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"create"}},
		{APIGroups: []string{"landlockgenprof.io"}, Resources: []string{"observations"}, Verbs: []string{"create"}},
		{APIGroups: []string{"landlockgenprof.io"}, Resources: []string{"observations/status"}, Verbs: []string{"get", "update", "patch"}},
	}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.RbacV1().RoleBindings("team-a").Create(ctx, &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "operator-alice", Namespace: "team-a"}, Subjects: []rbacv1.Subject{{Kind: "User", Name: "alice"}}, RoleRef: rbacv1.RoleRef{Kind: "Role", Name: "operator"}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, pod := range []*corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "alice-pod", Namespace: "team-a"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "busybox"}}}}, {ObjectMeta: metav1.ObjectMeta{Name: "bob-pod", Namespace: "team-b"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "busybox"}}}}} {
		if _, err := admin.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := admin.RbacV1().Roles("team-a").Create(ctx, &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "executor", Namespace: "team-a"}, Rules: []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"}},
		{APIGroups: []string{"apps"}, Resources: []string{"replicasets"}, Verbs: []string{"get"}},
		{APIGroups: []string{"landlockgenprof.io"}, Resources: []string{"observations"}, Verbs: []string{"get"}},
		{APIGroups: []string{"landlockgenprof.io"}, Resources: []string{"observations/status"}, Verbs: []string{"get", "update"}},
	}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.RbacV1().RoleBindings("team-a").Create(ctx, &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "executor", Namespace: "team-a"}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "landlock-genprof-observation-executor", Namespace: "landlock-genprof"}}, RoleRef: rbacv1.RoleRef{Kind: "Role", Name: "executor"}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	resources := []schema.GroupVersionResource{
		{Group: "", Version: "v1", Resource: "pods"},
		{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "observations"},
		{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "observationcontributionreceipts"},
		{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "traininghistories"},
		{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "securityprofileproposals"},
		{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "applyattempts"},
		{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "rollbackattempts"},
	}
	for _, subject := range []struct{ name, allowedNamespace, deniedNamespace string }{{"alice", "team-a", "team-b"}, {"bob", "team-b", "team-a"}} {
		client, err := NewImpersonatedClients(authorizationConfig, authn.Identity{Username: subject.name})
		if err != nil {
			t.Fatal(err)
		}
		for _, resource := range resources {
			if _, err := client.Dynamic.Resource(resource).Namespace(subject.allowedNamespace).List(ctx, metav1.ListOptions{}); err != nil {
				t.Fatalf("%s cannot list authorized %s: %v", subject.name, resource.Resource, err)
			}
			if _, err := client.Dynamic.Resource(resource).Namespace(subject.deniedNamespace).List(ctx, metav1.ListOptions{}); !apierrors.IsForbidden(err) {
				t.Fatalf("%s cross-namespace list for %s error = %v, want Forbidden", subject.name, resource.Resource, err)
			}
			if _, err := client.Dynamic.Resource(resource).Namespace(subject.allowedNamespace).Get(ctx, "missing", metav1.GetOptions{}); err != nil && !apierrors.IsNotFound(err) {
				t.Fatalf("%s authorized get for %s error = %v, want NotFound", subject.name, resource.Resource, err)
			}
			if _, err := client.Dynamic.Resource(resource).Namespace(subject.deniedNamespace).Get(ctx, "missing", metav1.GetOptions{}); !apierrors.IsForbidden(err) {
				t.Fatalf("%s cross-namespace get for %s error = %v, want Forbidden", subject.name, resource.Resource, err)
			}
		}
		proposalResource := schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "securityprofileproposals"}
		for _, operation := range []string{"review", "approve", "reject"} {
			if _, err := client.Dynamic.Resource(proposalResource).Namespace(subject.allowedNamespace).Get(ctx, "missing-"+operation, metav1.GetOptions{}); err != nil && !apierrors.IsNotFound(err) {
				t.Fatalf("%s authorized %s Proposal GET error = %v, want NotFound", subject.name, operation, err)
			}
			if _, err := client.Dynamic.Resource(proposalResource).Namespace(subject.deniedNamespace).Get(ctx, "missing-"+operation, metav1.GetOptions{}); !apierrors.IsForbidden(err) {
				t.Fatalf("%s cross-namespace %s Proposal GET error = %v, want Forbidden", subject.name, operation, err)
			}
			statusObject := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "landlockgenprof.io/v1alpha1", "kind": "SecurityProfileProposal", "metadata": map[string]interface{}{"name": "missing-" + operation, "namespace": subject.allowedNamespace}, "status": map[string]interface{}{"approvalState": "Draft"}}}
			if _, err := client.Dynamic.Resource(proposalResource).Namespace(subject.allowedNamespace).UpdateStatus(ctx, statusObject, metav1.UpdateOptions{}); err != nil && !apierrors.IsNotFound(err) {
				t.Fatalf("%s authorized %s Proposal status update error = %v, want NotFound", subject.name, operation, err)
			}
			if _, err := client.Dynamic.Resource(proposalResource).Namespace(subject.deniedNamespace).UpdateStatus(ctx, statusObject.DeepCopy(), metav1.UpdateOptions{}); !apierrors.IsForbidden(err) {
				t.Fatalf("%s cross-namespace %s Proposal status update error = %v, want Forbidden", subject.name, operation, err)
			}
		}
		for _, resource := range []string{"applyattempts", "rollbackattempts"} {
			gvr := schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: resource}
			object := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "landlockgenprof.io/v1alpha1", "kind": resource, "metadata": map[string]interface{}{"generateName": "g5-", "namespace": subject.allowedNamespace}, "spec": map[string]interface{}{}}}
			if _, err := client.Dynamic.Resource(gvr).Namespace(subject.allowedNamespace).Create(ctx, object, metav1.CreateOptions{}); apierrors.IsForbidden(err) {
				t.Fatalf("%s authorized %s create was Forbidden: %v", subject.name, resource, err)
			}
			if _, err := client.Dynamic.Resource(gvr).Namespace(subject.deniedNamespace).Create(ctx, object.DeepCopy(), metav1.CreateOptions{}); !apierrors.IsForbidden(err) {
				t.Fatalf("%s cross-namespace %s create error = %v, want Forbidden", subject.name, resource, err)
			}
		}
		caps, err := DiscoverCapabilities(ctx, client.Core, subject.allowedNamespace)
		if err != nil || !caps[WorkloadView] || !caps[ObservationView] || !caps[ProposalView] || !caps[HistoryView] {
			t.Fatalf("%s capability discovery = %#v, err=%v", subject.name, caps, err)
		}
	}
	alice, err := NewImpersonatedClients(authorizationConfig, authn.Identity{Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	validPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "created-by-alice", Namespace: "team-a"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "busybox"}}}}
	if _, err := alice.Core.CoreV1().Pods("team-a").Create(ctx, validPod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("authorized create failed: %v", err)
	}
	if _, err := alice.Core.CoreV1().Pods("team-b").Create(ctx, validPod.DeepCopy(), metav1.CreateOptions{}); !apierrors.IsForbidden(err) {
		t.Fatalf("cross-namespace pod create error = %v, want Forbidden", err)
	}
	obsResource := schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "observations"}
	minimalObservation := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "landlockgenprof.io/v1alpha1", "kind": "Observation", "metadata": map[string]interface{}{"name": "created-by-alice", "namespace": "team-a"}, "spec": map[string]interface{}{}}}
	if _, err := alice.Dynamic.Resource(obsResource).Namespace("team-a").Create(ctx, minimalObservation, metav1.CreateOptions{}); apierrors.IsForbidden(err) {
		t.Fatalf("authorized Observation create was Forbidden: %v", err)
	}
	if _, err := alice.Dynamic.Resource(obsResource).Namespace("team-b").Create(ctx, minimalObservation.DeepCopy(), metav1.CreateOptions{}); !apierrors.IsForbidden(err) {
		t.Fatalf("cross-namespace Observation create error = %v, want Forbidden", err)
	}
	if _, err := alice.Dynamic.Resource(schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "observations"}).Namespace("team-a").UpdateStatus(ctx, minimalObservation, metav1.UpdateOptions{}); apierrors.IsForbidden(err) {
		t.Fatalf("authorized Observation status update was Forbidden: %v", err)
	}
	if _, err := alice.Dynamic.Resource(obsResource).Namespace("team-b").UpdateStatus(ctx, minimalObservation.DeepCopy(), metav1.UpdateOptions{}); !apierrors.IsForbidden(err) {
		t.Fatalf("cross-namespace Observation status update error = %v, want Forbidden", err)
	}
	executorConfig := rest.CopyConfig(authorizationConfig)
	executorConfig.Impersonate.UserName = "system:serviceaccount:landlock-genprof:landlock-genprof-observation-executor"
	executorCore, err := kubernetes.NewForConfig(executorConfig)
	if err != nil {
		t.Fatal(err)
	}
	executorDynamic, err := dynamic.NewForConfig(executorConfig)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executorCore.CoreV1().Pods("team-a").List(ctx, metav1.ListOptions{}); err != nil {
		t.Fatalf("executor cannot read target Pods: %v", err)
	}
	if _, err := executorDynamic.Resource(schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "observations"}).Namespace("team-a").Get(ctx, "missing", metav1.GetOptions{}); err == nil {
		t.Fatal("executor unexpectedly bypassed Observation authorization")
	}
	if _, err := executorDynamic.Resource(schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "securityprofileproposals"}).Namespace("team-a").List(ctx, metav1.ListOptions{}); err == nil {
		t.Fatal("executor unexpectedly listed proposals")
	}
	if _, err := executorCore.AppsV1().Deployments("team-a").Patch(ctx, "missing", types.MergePatchType, []byte(`{"spec":{}}`), metav1.PatchOptions{}); err == nil {
		t.Fatal("executor unexpectedly patched a workload")
	}

	backendConfig := rest.CopyConfig(authorizationConfig)
	backendConfig.Impersonate.UserName = "system:serviceaccount:landlock-genprof:landlock-genprof-operations-center"
	backendCore, err := kubernetes.NewForConfig(backendConfig)
	if err != nil {
		t.Fatal(err)
	}
	backendDynamic, err := dynamic.NewForConfig(backendConfig)
	if err != nil {
		t.Fatal(err)
	}
	backendProposal := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "landlockgenprof.io/v1alpha1", "kind": "SecurityProfileProposal", "metadata": map[string]interface{}{"name": "backend-proposal", "namespace": "team-a"}, "status": map[string]interface{}{"approvalState": "Draft"}}}
	if _, err := backendDynamic.Resource(schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "securityprofileproposals"}).Namespace("team-a").UpdateStatus(ctx, backendProposal, metav1.UpdateOptions{}); !apierrors.IsForbidden(err) {
		t.Fatalf("backend direct Proposal status mutation error = %v, want Forbidden", err)
	}
	for _, resource := range []string{"applyattempts", "rollbackattempts"} {
		gvr := schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: resource}
		object := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "landlockgenprof.io/v1alpha1", "kind": resource, "metadata": map[string]interface{}{"generateName": "backend-", "namespace": "team-a"}, "spec": map[string]interface{}{}}}
		if _, err := backendDynamic.Resource(gvr).Namespace("team-a").Create(ctx, object, metav1.CreateOptions{}); !apierrors.IsForbidden(err) {
			t.Fatalf("backend direct %s mutation error = %v, want Forbidden", resource, err)
		}
	}
	_ = backendCore
}

func TestExplicitNamespaceWorksWithoutNamespaceListAndRevocationIsObserved(t *testing.T) {
	ctx := context.Background()
	admin, err := kubernetes.NewForConfig(authorizationConfig)
	if err != nil {
		t.Fatal(err)
	}
	namespace := "explicit-only"
	if _, err := admin.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.RbacV1().Roles(namespace).Create(ctx, &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "explicit-viewer", Namespace: namespace}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"list"}}}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	binding, err := admin.RbacV1().RoleBindings(namespace).Create(ctx, &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "explicit-viewer", Namespace: namespace}, Subjects: []rbacv1.Subject{{Kind: "User", Name: "explicit-user"}}, RoleRef: rbacv1.RoleRef{Kind: "Role", Name: "explicit-viewer"}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewImpersonatedClients(authorizationConfig, authn.Identity{Username: "explicit-user"})
	if err != nil {
		t.Fatal(err)
	}
	discovery, err := DiscoverNamespaces(ctx, client.Core)
	if err != nil {
		t.Fatal(err)
	}
	if discovery.Mode != NamespaceExplicitOnly || discovery.CanListNamespaces || len(discovery.Namespaces) != 0 {
		t.Fatalf("discovery=%+v, want explicit-only", discovery)
	}
	caps, err := ExplicitNamespaceAccess(ctx, client.Core, namespace)
	if err != nil || !caps[WorkloadView] {
		t.Fatalf("explicit capabilities=%+v err=%v", caps, err)
	}
	if err := admin.RbacV1().RoleBindings(namespace).Delete(ctx, binding.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := ExplicitNamespaceAccess(ctx, client.Core, namespace); err != ErrNamespaceUnavailable {
		t.Fatalf("revoked explicit access error=%v, want ErrNamespaceUnavailable", err)
	}
}
