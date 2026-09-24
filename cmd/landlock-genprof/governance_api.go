package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/applyproposal"
	"github.com/idriss-eliguene/landlock-genprof/internal/attempt"
	"github.com/idriss-eliguene/landlock-genprof/internal/authz"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
	rollbackapp "github.com/idriss-eliguene/landlock-genprof/internal/rollback"
	"k8s.io/client-go/dynamic"
)

// governanceRequest contains only operation-specific semantic input. Identity,
// namespace, kubeconfig, and target are taken from the authenticated request
// and the named durable object, never from the browser.
type governanceRequest struct {
	Reason                  string `json:"reason,omitempty"`
	ExpectedDigest          string `json:"expectedDigest,omitempty"`
	ExpectedResourceVersion string `json:"expectedResourceVersion,omitempty"`
}

type governanceResponse struct {
	Operation      string `json:"operation"`
	Namespace      string `json:"namespace"`
	Resource       string `json:"resource"`
	PreviousState  string `json:"previousState,omitempty"`
	ResultingState string `json:"resultingState"`
	Attempt        string `json:"attempt,omitempty"`
	Actor          string `json:"actor"`
	Success        bool   `json:"success"`
	Message        string `json:"message,omitempty"`
}

func (s *workbenchServer) handleGovernanceProposal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeGovernanceError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "named governance operations require POST")
		return
	}
	if !s.authenticated || s.dynamic == nil || s.requestIdentity.Username == "" {
		writeGovernanceError(w, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "authenticated governance operations are required")
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/governance/proposals/"), "/")
	if len(parts) != 2 || parts[0] == "" {
		writeGovernanceError(w, http.StatusBadRequest, "INVALID_REQUEST", "proposal name and named operation are required")
		return
	}
	if parts[1] == "apply" {
		s.handleGovernanceApply(w, r)
		return
	}
	name, operation := parts[0], parts[1]
	capability := authz.ProposalReview
	if operation == "approve" || operation == "reject" {
		capability = authz.ProposalApprove
	}
	if operation != "review" && operation != "approve" && operation != "reject" {
		writeGovernanceError(w, http.StatusNotFound, "NOT_FOUND", "unknown governance operation")
		return
	}
	if !s.capabilityAllowed(r.Context(), capability) {
		writeGovernanceError(w, http.StatusForbidden, "AUTHORIZATION_DENIED", "the authenticated identity lacks the required capability")
		return
	}
	var request governanceRequest
	if r.Body != nil && r.Body != http.NoBody {
		if err := decodeJSON(w, r, &request); err != nil {
			writeGovernanceError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid governance request")
			return
		}
	}
	if request.ExpectedResourceVersion == "" {
		writeGovernanceError(w, http.StatusPreconditionRequired, "PRECONDITION_REQUIRED", "expectedResourceVersion is required")
		return
	}
	namespace := s.reads.SessionIdentity().Namespace
	// Every named governance transition operates only on a Proposal that the
	// certified domain decoder accepts. In particular, Reject must not become
	// a side door that mutates a Proposal excluded by the G4 projection.
	if _, _, resourceVersion, err := proposal.GetWithIdentityAndResourceVersion(r.Context(), s.dynamic, namespace, name); err != nil {
		writeGovernanceApplicationError(w, err)
		return
	} else if resourceVersion != request.ExpectedResourceVersion {
		writeGovernanceError(w, http.StatusConflict, "CONFLICT", "proposal resourceVersion changed before governance operation")
		return
	}
	previous, statusResourceVersion, err := proposal.GetStatusWithResourceVersion(r.Context(), s.dynamic, namespace, name)
	if err != nil {
		writeGovernanceApplicationError(w, err)
		return
	}
	if statusResourceVersion != request.ExpectedResourceVersion {
		writeGovernanceError(w, http.StatusConflict, "CONFLICT", "proposal resourceVersion changed before governance operation")
		return
	}
	var operationErr error
	switch operation {
	case "review":
		operationErr = proposal.MarkReviewedByVersion(r.Context(), s.dynamic, namespace, name, s.requestIdentity.Username, request.ExpectedResourceVersion)
	case "approve":
		operationErr = proposal.SetApprovalStateByVersion(r.Context(), s.dynamic, namespace, name, proposal.ApprovalApproved, request.Reason, request.ExpectedDigest, s.requestIdentity.Username, request.ExpectedResourceVersion)
	case "reject":
		operationErr = proposal.SetApprovalStateByVersion(r.Context(), s.dynamic, namespace, name, proposal.ApprovalRejected, request.Reason, "", s.requestIdentity.Username, request.ExpectedResourceVersion)
	}
	if operationErr != nil {
		writeGovernanceApplicationError(w, operationErr)
		return
	}
	result, err := proposal.GetStatus(r.Context(), s.dynamic, namespace, name)
	if err != nil {
		writeGovernanceApplicationError(w, err)
		return
	}
	writeGovernanceJSON(w, governanceResponse{
		Operation: operation, Namespace: namespace, Resource: "SecurityProfileProposal/" + name,
		PreviousState: string(previous.ApprovalState), ResultingState: string(result.ApprovalState),
		Actor: s.requestIdentity.Username, Success: true,
	})
}

func (s *workbenchServer) handleGovernanceApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeGovernanceError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Apply requires POST")
		return
	}
	if !s.authenticated || s.dynamic == nil || s.requestIdentity.Username == "" {
		writeGovernanceError(w, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "authenticated governance operations are required")
		return
	}
	tail := strings.TrimPrefix(r.URL.Path, "/api/governance/proposals/")
	parts := strings.Split(tail, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "apply" {
		writeGovernanceError(w, http.StatusBadRequest, "INVALID_REQUEST", "proposal name is required")
		return
	}
	name := parts[0]
	if !s.capabilityAllowed(r.Context(), authz.ProposalApply) {
		writeGovernanceError(w, http.StatusForbidden, "AUTHORIZATION_DENIED", "the authenticated identity lacks proposal.apply")
		return
	}
	var request governanceRequest
	if r.Body != nil && r.Body != http.NoBody {
		if err := decodeJSON(w, r, &request); err != nil {
			writeGovernanceError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid governance request")
			return
		}
	}
	if request.ExpectedResourceVersion == "" {
		writeGovernanceError(w, http.StatusPreconditionRequired, "PRECONDITION_REQUIRED", "expectedResourceVersion is required")
		return
	}
	namespace := s.reads.SessionIdentity().Namespace
	var output bytes.Buffer
	err := applyproposal.Run(r.Context(), &output, strings.NewReader(""), applyproposal.Options{
		Namespace: namespace, Yes: true, OperatorIdentity: s.requestIdentity.Username, ExpectedResourceVersion: request.ExpectedResourceVersion, ReadinessTimeout: 2 * time.Minute,
	}, name, false, applyproposal.Dependencies{
		NewDynamicClient:               func() (dynamic.Interface, error) { return s.dynamic, nil },
		ClusterScopedClient:            func() (dynamic.Interface, error) { return profileClientOrUnavailable(s.profileDynamic) },
		AuthorizeClusterScopedArtifact: authorizeClusterScopedProfile,
		SaveAttemptStatus:              attempt.SaveStatus,
		CreateAttempt:                  attempt.Create,
		ReadApplyResource:              k8s.ReadApplyResource,
		ApplyManifestObserved: func(ctx context.Context, client dynamic.Interface, ns, content string, guard k8s.ApplyGuard) (k8s.MutationObservation, error) {
			return k8s.ApplyWithGuardObserved(ctx, client, ns, content, guard)
		},
	})
	if err != nil {
		writeGovernanceApplicationError(w, err)
		return
	}
	writeGovernanceJSON(w, governanceResponse{
		Operation: "apply", Namespace: namespace, Resource: "SecurityProfileProposal/" + name,
		ResultingState: "SUCCEEDED", Attempt: custodyAttemptFromOutput(output.String(), "ApplyAttempt:"),
		Actor: s.requestIdentity.Username, Success: true, Message: "application completed",
	})
}

func (s *workbenchServer) handleGovernanceRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeGovernanceError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Rollback requires POST")
		return
	}
	if !s.authenticated || s.dynamic == nil || s.requestIdentity.Username == "" {
		writeGovernanceError(w, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "authenticated governance operations are required")
		return
	}
	tail := strings.TrimPrefix(r.URL.Path, "/api/governance/apply-attempts/")
	parts := strings.Split(tail, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "rollback" {
		writeGovernanceError(w, http.StatusBadRequest, "INVALID_REQUEST", "ApplyAttempt name is required")
		return
	}
	name := parts[0]
	if !s.capabilityAllowed(r.Context(), authz.RollbackExecute) {
		writeGovernanceError(w, http.StatusForbidden, "AUTHORIZATION_DENIED", "the authenticated identity lacks rollback.execute")
		return
	}
	var request governanceRequest
	if r.Body != nil && r.Body != http.NoBody {
		if err := decodeJSON(w, r, &request); err != nil {
			writeGovernanceError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid governance request")
			return
		}
	}
	if request.ExpectedResourceVersion == "" {
		writeGovernanceError(w, http.StatusPreconditionRequired, "PRECONDITION_REQUIRED", "expectedResourceVersion is required")
		return
	}
	namespace := s.reads.SessionIdentity().Namespace
	var output bytes.Buffer
	err := rollbackapp.Run(r.Context(), &output, strings.NewReader(""), rollbackapp.Options{Namespace: namespace, Yes: true, OperatorIdentity: s.requestIdentity.Username, ExpectedResourceVersion: request.ExpectedResourceVersion}, name, rollbackapp.Dependencies{
		NewDynamicClient:              func() (dynamic.Interface, error) { return s.dynamic, nil },
		ClusterScopedClient:           func() (dynamic.Interface, error) { return profileClientOrUnavailable(s.profileDynamic) },
		AuthorizeClusterScopedInverse: authorizeClusterScopedInverse,
		CreateRollbackAttempt:         attempt.CreateRollback,
		SaveRollbackAttemptStatus:     attempt.SaveRollbackStatus,
		ExecuteInverse:                executeInverse,
		InverseFailureResult:          inverseFailureResult,
	})
	if err != nil {
		writeGovernanceApplicationError(w, err)
		return
	}
	writeGovernanceJSON(w, governanceResponse{
		Operation: "rollback", Namespace: namespace, Resource: "ApplyAttempt/" + name,
		ResultingState: "SUCCEEDED", Attempt: custodyAttemptFromOutput(output.String(), "RollbackAttempt:"),
		Actor: s.requestIdentity.Username, Success: true, Message: "rollback completed",
	})
}

func (s *workbenchServer) capabilityAllowed(ctx context.Context, capability authz.Capability) bool {
	if s.discoverCaps == nil || !s.applicationCapabilityAllowed(capability) {
		return false
	}
	capabilities, err := s.discoverCaps(ctx, s.reads.SessionIdentity().Namespace)
	return err == nil && capabilities[capability]
}

func custodyAttemptFromOutput(output, prefix string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func writeGovernanceJSON(w http.ResponseWriter, response governanceResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func writeGovernanceError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
}

func writeGovernanceApplicationError(w http.ResponseWriter, err error) {
	message := err.Error()
	status, code := http.StatusInternalServerError, "UNKNOWN_INTERNAL"
	switch {
	case strings.Contains(strings.ToLower(message), "forbidden"), strings.Contains(strings.ToLower(message), "unauthorized"):
		status, code = http.StatusForbidden, "AUTHORIZATION_DENIED"
	case strings.Contains(message, "not found"):
		status, code = http.StatusNotFound, "NOT_FOUND"
	case strings.Contains(message, "expected candidate digest mismatch"), strings.Contains(message, "preflight failed"):
		status, code = http.StatusPreconditionFailed, "PRECONDITION_FAILED"
	case strings.Contains(message, "resourceVersion changed"):
		status, code = http.StatusConflict, "CONFLICT"
	case strings.Contains(message, "conflict"):
		status, code = http.StatusConflict, "CONFLICT"
	case strings.Contains(message, "REFUSE_"):
		status, code = http.StatusConflict, "INVALID_TRANSITION"
	case strings.Contains(message, "invalid") || strings.Contains(message, "no spec") || strings.Contains(message, "requires"):
		status, code = http.StatusUnprocessableEntity, "MALFORMED_OBJECT"
	}
	writeGovernanceError(w, status, code, boundedGovernanceMessage(message))
}

func boundedGovernanceMessage(message string) string {
	const max = 512
	if len(message) > max {
		return message[:max]
	}
	return message
}
