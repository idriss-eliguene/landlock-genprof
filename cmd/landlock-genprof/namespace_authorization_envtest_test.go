//go:build envtest

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/authn"
	"github.com/idriss-eliguene/landlock-genprof/internal/authz"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var namespaceAuthorizationProposalGVR = schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "securityprofileproposals"}

// TestOwnershipMismatchThroughRealAPI exercises the corrected negative
// fixture against a real kube-apiserver. The proposal, target workload,
// namespace-local RoleBindings, UID and resourceVersion all come from the
// API server; no status or identity field is forged after creation.
//
// This intentionally tests two independent denial paths:
//   - an identity bound only in team-b cannot operate on team-a;
//   - a team-a reviewer with Kubernetes status permission still cannot approve
//     because proposal.review and proposal.approve are application roles.
//
// The test does not claim node-level enforcement or SPO realization. It proves
// that an unauthorized governance request is rejected before any mutation.
func TestOwnershipMismatchThroughRealAPI(t *testing.T) {
	ctx := context.Background()
	adminCore, err := kubernetes.NewForConfig(e2eConfig)
	if err != nil {
		t.Fatal(err)
	}
	adminDynamic, err := dynamic.NewForConfig(e2eConfig)
	if err != nil {
		t.Fatal(err)
	}

	teamA := "authz-owner-a"
	teamB := "authz-owner-b"
	for _, namespace := range []string{teamA, teamB} {
		if _, err := adminCore.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: teamA},
		Spec:       appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "api", Image: "busybox"}}}}},
	}
	if _, err := adminCore.AppsV1().Deployments(teamA).Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	proposalObject, _ := validGovernanceProposal(t)
	proposalObject.SetName("ownership-mismatch")
	proposalObject.SetNamespace(teamA)
	proposalObject.SetUID("")
	proposalObject.SetResourceVersion("")
	created, err := adminDynamic.Resource(namespaceAuthorizationProposalGVR).Namespace(teamA).Create(ctx, proposalObject, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("creating valid proposal fixture: %v", err)
	}
	if created.GetUID() == "" || created.GetResourceVersion() == "" {
		t.Fatalf("API server did not assign proposal custody metadata: uid=%q resourceVersion=%q", created.GetUID(), created.GetResourceVersion())
	}

	// Both users receive only the status permission in their own namespace.
	// Kubernetes therefore exposes the same status verb for review and approve;
	// the application capability boundary must provide the distinction.
	for _, namespace := range []string{teamA, teamB} {
		role := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "governance", Namespace: namespace}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{"landlockgenprof.io"}, Resources: []string{"securityprofileproposals", "securityprofileproposals/status"}, Verbs: []string{"get", "list", "update", "patch"}}}}
		if _, err := adminCore.RbacV1().Roles(namespace).Create(ctx, role, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := bindUser(ctx, adminCore, teamA, "reviewer-only", "governance"); err != nil {
		t.Fatal(err)
	}
	if err := bindUser(ctx, adminCore, teamB, "cross-namespace-approver", "governance"); err != nil {
		t.Fatal(err)
	}

	reviewer := addEnvtestUser(t, "reviewer-only", []string{"security-reviewers"})
	crossNamespaceApprover := addEnvtestUser(t, "cross-namespace-approver", []string{"security-approvers"})

	// A reviewer in the owning namespace may review, but may not approve.
	reviewerServer := governanceServerForUser(t, reviewer, teamA, authn.Identity{Username: "reviewer-only", Groups: []string{"security-reviewers"}}, []string{"security-reviewers"}, []string{"security-approvers"})
	reviewResponse := invokeGovernance(reviewerServer, "/api/governance/proposals/ownership-mismatch/review", `{"expectedResourceVersion":"`+created.GetResourceVersion()+`"}`)
	if reviewResponse.Code != http.StatusOK {
		t.Fatalf("authorized review status=%d body=%s", reviewResponse.Code, reviewResponse.Body.String())
	}
	updated, err := adminDynamic.Resource(namespaceAuthorizationProposalGVR).Namespace(teamA).Get(ctx, "ownership-mismatch", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	statusAfterReview, err := proposal.GetStatus(ctx, adminDynamic, teamA, "ownership-mismatch")
	if err != nil {
		t.Fatal(err)
	}
	approveResponse := invokeGovernance(reviewerServer, "/api/governance/proposals/ownership-mismatch/approve", `{"expectedDigest":"`+proposalDigest(t, proposalObject)+`","expectedResourceVersion":"`+updated.GetResourceVersion()+`"}`)
	if approveResponse.Code != http.StatusForbidden {
		t.Fatalf("review-only approval status=%d body=%s, want 403", approveResponse.Code, approveResponse.Body.String())
	}

	// An approver bound only in team-b cannot access the team-a proposal. The
	// SSAR is real and is evaluated by the envtest API server before the handler
	// reads or mutates the proposal.
	crossServer := governanceServerForUser(t, crossNamespaceApprover, teamA, authn.Identity{Username: "cross-namespace-approver", Groups: []string{"security-approvers"}}, []string{"security-reviewers"}, []string{"security-approvers"})
	unauthorized := invokeGovernance(crossServer, "/api/governance/proposals/ownership-mismatch/approve", `{"expectedDigest":"`+proposalDigest(t, proposalObject)+`","expectedResourceVersion":"`+updated.GetResourceVersion()+`"}`)
	if unauthorized.Code != http.StatusForbidden {
		t.Fatalf("cross-namespace approval status=%d body=%s, want 403", unauthorized.Code, unauthorized.Body.String())
	}

	statusAfter, err := proposal.GetStatus(ctx, adminDynamic, teamA, "ownership-mismatch")
	if err != nil {
		t.Fatal(err)
	}
	if statusAfter.ApprovalState != statusAfterReview.ApprovalState || statusAfter.ReviewedBy != statusAfterReview.ReviewedBy || statusAfter.ApprovedBy != statusAfterReview.ApprovedBy {
		t.Fatalf("unauthorized request changed approval state: before=%+v after=%+v", statusAfterReview, statusAfter)
	}
	applyAttempts, err := adminDynamic.Resource(schema.GroupVersionResource{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "applyattempts"}).Namespace(teamA).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(applyAttempts.Items) != 0 {
		t.Fatalf("unauthorized request created %d ApplyAttempt objects", len(applyAttempts.Items))
	}
}

func addEnvtestUser(t *testing.T, username string, groups []string) *rest.Config {
	t.Helper()
	user, err := e2eEnv.AddUser(envtest.User{Name: username, Groups: groups}, nil)
	if err != nil {
		t.Fatalf("adding envtest user %q: %v", username, err)
	}
	return user.Config()
}

func governanceServerForUser(t *testing.T, config *rest.Config, namespace string, identity authn.Identity, reviewGroups, approverGroups []string) *workbenchServer {
	t.Helper()
	core, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	reads, err := k8s.NewReadSessionForClients(core, dynamicClient, discoveryClient, namespace)
	if err != nil {
		t.Fatal(err)
	}
	return &workbenchServer{
		reads: reads, dynamic: dynamicClient, authenticated: true, requestIdentity: identity,
		reviewGroups: reviewGroups, approverGroups: approverGroups,
		discoverCaps: func(ctx context.Context, namespace string) (map[authz.Capability]bool, error) {
			return authz.DiscoverCapabilities(ctx, core, namespace)
		},
	}
}

func bindUser(ctx context.Context, core kubernetes.Interface, namespace, username, roleName string) error {
	_, err := core.RbacV1().RoleBindings(namespace).Create(ctx, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: roleName + "-" + username, Namespace: namespace},
		Subjects:   []rbacv1.Subject{{Kind: "User", Name: username}},
		RoleRef:    rbacv1.RoleRef{Kind: "Role", Name: roleName},
	}, metav1.CreateOptions{})
	return err
}

func invokeGovernance(server *workbenchServer, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	response := httptest.NewRecorder()
	server.mux().ServeHTTP(response, request)
	return response
}

func proposalDigest(t *testing.T, object *unstructured.Unstructured) string {
	t.Helper()
	var spec proposal.Spec
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object["spec"].(map[string]interface{}), &spec); err != nil {
		t.Fatal(err)
	}
	digest, err := proposal.CandidateDigestV2(mustCandidate(spec))
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
