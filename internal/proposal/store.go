// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package proposal

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/retry"
)

var ErrProposalPersistenceConflict = errors.New("proposal persistence conflict")

const (
	apiGroup   = "landlockgenprof.io"
	apiVersion = "v1alpha1"
	kind       = "SecurityProfileProposal"
)

// securityProfileProposalGVR must match
// deploy/crd-securityprofileproposal.yaml's group/version/plural
// exactly — there's no code-level link between them, this is it. Same
// group as internal/history's trainingHistoryGVR: SecurityProfileProposal
// is a second Kind under landlockgenprof.io, not a reason for a second
// API group.
var securityProfileProposalGVR = schema.GroupVersionResource{
	Group:    apiGroup,
	Version:  apiVersion,
	Resource: "securityprofileproposals",
}

// Save creates or updates the SecurityProfileProposal object for name in
// namespace — a plain overwrite-on-rerun snapshot, not an accumulation
// the way internal/history.Merge is: a proposal represents "the latest
// generated recommendation," matching how the CLI's local files already
// behave (trace overwrites <pod>-profile.yaml on every run too).
//
// Built via runtime.DefaultUnstructuredConverter, not a hand-rolled map
// the way internal/history/store.go's toUnstructured is — appropriate
// there for Record's flat, hand-rolled shape, but the wrong tool for
// Spec's nested real Kubernetes/PodLock/seccomp API types (confirmed via
// k8s.io/apimachinery/pkg/runtime/converter.go: this is the same
// converter client-go itself uses for this exact purpose).
func Save(ctx context.Context, client dynamic.Interface, namespace, name string, spec Spec) error {
	if err := ValidateProposalSpec(spec); err != nil {
		return fmt.Errorf("invalid proposal spec for %s/%s: %w", namespace, name, err)
	}
	if spec.TargetBinding != nil {
		if _, err := spec.TargetBinding.GovernedTarget(spec.Container); err != nil {
			return fmt.Errorf("invalid canonical target binding for %s/%s: %w", namespace, name, err)
		}
	}
	resource := client.Resource(securityProfileProposalGVR).Namespace(namespace)

	specMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&spec)
	if err != nil {
		return fmt.Errorf("converting proposal spec for %s/%s: %w", namespace, name, err)
	}

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiGroup + "/" + apiVersion,
		"kind":       kind,
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": specMap,
	}}

	existing, err := resource.Get(ctx, name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		created, err := resource.Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				return convergeAfterProposalWriteRace(ctx, resource, namespace, name, spec, err)
			}
			return fmt.Errorf("creating SecurityProfileProposal %s/%s: %w", namespace, name, err)
		}
		// The status subresource (deploy/crd-securityprofileproposal.yaml)
		// means Create above never persists .status, regardless of what
		// obj contained — a separate UpdateStatus call is required even
		// for the very first write. A hard failure here, matching this
		// function's own existing "publishing the proposal is mandatory"
		// behavior on the Create/Update calls above and in
		// cmd/landlock-genprof/trace.go — not best-effort: a proposal
		// silently missing its Draft status would just be confusing
		// under review/approve, not a harmless gap. (GetStatus/
		// MarkReviewed/SetApprovalState still treat a blank ApprovalState
		// as Draft regardless, for any proposal that predates this CRD
		// version and was never backfilled.)
		if err := setStatus(ctx, resource, created, Status{ApprovalState: ApprovalDraft}); err != nil {
			return fmt.Errorf("setting initial Draft status on SecurityProfileProposal %s/%s: %w", namespace, name, err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("fetching SecurityProfileProposal %s/%s before update: %w", namespace, name, err)
	}

	obj.SetResourceVersion(existing.GetResourceVersion())
	// UID is immutable object identity. Carry it on the reconstructed update
	// object so persistence adapters (including the fake client used by tests)
	// retain the same Proposal identity across an intentional Spec overwrite.
	obj.SetUID(existing.GetUID())
	// Carry the existing .status over explicitly rather than relying on
	// the status subresource to silently preserve it server-side —
	// correct either way against a real API server (which ignores
	// .status on this endpoint regardless of what's sent), but doesn't
	// depend on that specific subresource-isolation guarantee actually
	// holding (confirmed live: client-go's fake dynamic client used in
	// tests does NOT enforce it, and would otherwise wipe an approval
	// decision on every `trace` re-run in exactly the scenario this
	// whole status-subresource split exists to prevent).
	if existingStatus, found := existing.Object["status"]; found {
		obj.Object["status"] = existingStatus
	}
	if _, err := resource.Update(ctx, obj, metav1.UpdateOptions{}); err != nil {
		if apierrors.IsConflict(err) {
			return convergeAfterProposalWriteRace(ctx, resource, namespace, name, spec, err)
		}
		return fmt.Errorf("updating SecurityProfileProposal %s/%s: %w", namespace, name, err)
	}
	return nil
}

func proposalSpecFromObject(obj *unstructured.Unstructured) (Spec, error) {
	if obj == nil {
		return Spec{}, fmt.Errorf("SecurityProfileProposal is nil")
	}
	specMap, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil {
		return Spec{}, fmt.Errorf("reading Proposal spec: %w", err)
	}
	if !found {
		return Spec{}, fmt.Errorf("SecurityProfileProposal has no spec")
	}
	var spec Spec
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(specMap, &spec); err != nil {
		return Spec{}, fmt.Errorf("converting Proposal spec: %w", err)
	}
	if err := ValidateProposalSpec(spec); err != nil {
		return Spec{}, fmt.Errorf("invalid Proposal spec: %w", err)
	}
	return spec, nil
}

// proposalSpecsEquivalent compares every persisted Spec field. The only
// normalization is the certified absent-version-to-v1 compatibility rule.
func proposalSpecsEquivalent(left, right Spec) (bool, error) {
	toMap := func(spec Spec) (map[string]interface{}, error) {
		version, err := normalizedCandidateVersion(spec.CandidateVersion)
		if err != nil {
			return nil, err
		}
		spec.CandidateVersion = version
		return runtime.DefaultUnstructuredConverter.ToUnstructured(&spec)
	}
	leftMap, err := toMap(left)
	if err != nil {
		return false, err
	}
	rightMap, err := toMap(right)
	if err != nil {
		return false, err
	}
	// GeneratedAt identifies the derivation invocation, not persisted
	// proposal content. Concurrent equivalent generation calls can straddle
	// an RFC3339-second boundary; it must not turn an otherwise identical
	// proposal into a competing Spec. Every content-bearing Spec field remains
	// in the comparison.
	delete(leftMap, "generatedAt")
	delete(rightMap, "generatedAt")
	return equality.Semantic.DeepEqual(leftMap, rightMap), nil
}

// convergeAfterProposalWriteRace only accepts an exact desired-Spec match
// observed by a fresh authoritative read. It never overwrites competing
// content and never consults status as convergence authority.
func convergeAfterProposalWriteRace(ctx context.Context, resource dynamic.ResourceInterface, namespace, name string, desired Spec, cause error) error {
	fresh, err := resource.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("proposal persistence race reread %s/%s: %w", namespace, name, err)
	}
	actual, err := proposalSpecFromObject(fresh)
	if err != nil {
		return fmt.Errorf("proposal persistence race found invalid %s/%s: %w", namespace, name, err)
	}
	equivalent, err := proposalSpecsEquivalent(desired, actual)
	if err != nil {
		return fmt.Errorf("comparing concurrent Proposal specs %s/%s: %w", namespace, name, err)
	}
	if equivalent {
		return nil
	}
	return fmt.Errorf("%w for SecurityProfileProposal %s/%s: %w", ErrProposalPersistenceConflict, namespace, name, cause)
}

// Get fetches the SecurityProfileProposal for name in namespace, or
// returns (nil, nil) if it doesn't exist yet. No CLI-facing read path
// uses this today — kept for round-trip testability and future reuse
// (e.g. a future `landlock-genprof proposal get` subcommand), mirroring
// internal/history.Get's own shape.
func Get(ctx context.Context, client dynamic.Interface, namespace, name string) (*Spec, error) {
	obj, err := client.Resource(securityProfileProposalGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetching SecurityProfileProposal %s/%s: %w", namespace, name, err)
	}

	specMap, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil {
		return nil, fmt.Errorf("reading spec from SecurityProfileProposal %s/%s: %w", namespace, name, err)
	}
	if !found {
		return &Spec{}, nil
	}

	var spec Spec
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(specMap, &spec); err != nil {
		return nil, fmt.Errorf("converting spec from SecurityProfileProposal %s/%s: %w", namespace, name, err)
	}
	if err := ValidateProposalSpec(spec); err != nil {
		return nil, fmt.Errorf("invalid proposal spec in SecurityProfileProposal %s/%s: %w", namespace, name, err)
	}
	return &spec, nil
}

// GetWithIdentity returns the proposal spec together with Kubernetes object
// identity. The UID is required to bind durable apply custody to this exact
// proposal rather than merely to a reusable namespace/name.
func GetWithIdentity(ctx context.Context, client dynamic.Interface, namespace, name string) (*Spec, string, error) {
	obj, err := client.Resource(securityProfileProposalGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("fetching SecurityProfileProposal %s/%s: %w", namespace, name, err)
	}
	specMap, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil {
		return nil, "", fmt.Errorf("reading spec from SecurityProfileProposal %s/%s: %w", namespace, name, err)
	}
	if !found {
		return nil, "", fmt.Errorf("securityprofileproposal %s/%s has no spec", namespace, name)
	}
	var spec Spec
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(specMap, &spec); err != nil {
		return nil, "", fmt.Errorf("converting spec from SecurityProfileProposal %s/%s: %w", namespace, name, err)
	}
	if err := ValidateProposalSpec(spec); err != nil {
		return nil, "", fmt.Errorf("invalid proposal spec in SecurityProfileProposal %s/%s: %w", namespace, name, err)
	}
	return &spec, string(obj.GetUID()), nil
}

// GetStatus fetches the approval status for name in namespace. A
// SecurityProfileProposal that exists but has no .status yet (e.g. one
// Save couldn't finish stamping with Draft, see Save's own comment on
// that) is treated as ApprovalDraft, not an error — Draft is what a
// blank status means everywhere else in this package too. Returns
// (nil, nil), like Get, if the proposal itself doesn't exist.
func GetStatus(ctx context.Context, client dynamic.Interface, namespace, name string) (*Status, error) {
	obj, err := client.Resource(securityProfileProposalGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetching SecurityProfileProposal %s/%s: %w", namespace, name, err)
	}
	return statusFromObject(obj)
}

// ListItem summarizes one SecurityProfileProposal for `policy list` —
// name and current approval status only, not the full Spec (large,
// rendered-artifact content nobody needs for a list view).
type ListItem struct {
	Name   string
	Status Status
}

// List returns every SecurityProfileProposal in namespace, sorted by
// name for deterministic output. The one operation this package didn't
// have before `policy list`/`policy status` needed it — every other
// function here is a by-name Get/Save, since review/approve/reject only
// ever operate on one proposal at a time.
func List(ctx context.Context, client dynamic.Interface, namespace string) ([]ListItem, error) {
	list, err := client.Resource(securityProfileProposalGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing SecurityProfileProposals in %s: %w", namespace, err)
	}

	items := make([]ListItem, 0, len(list.Items))
	for i := range list.Items {
		status, err := statusFromObject(&list.Items[i])
		if err != nil {
			return nil, err
		}
		items = append(items, ListItem{Name: list.Items[i].GetName(), Status: *status})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func statusFromObject(obj *unstructured.Unstructured) (*Status, error) {
	statusMap, found, err := unstructured.NestedMap(obj.Object, "status")
	if err != nil {
		return nil, fmt.Errorf("reading status from SecurityProfileProposal %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	if !found {
		return &Status{ApprovalState: ApprovalDraft}, nil
	}

	var status Status
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(statusMap, &status); err != nil {
		return nil, fmt.Errorf("converting status from SecurityProfileProposal %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	if err := status.LastApprovalSnapshot.Validate(); err != nil {
		return nil, fmt.Errorf("invalid approval custody in SecurityProfileProposal %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	if status.ApprovalState == "" {
		status.ApprovalState = ApprovalDraft
	}
	return &status, nil
}

// setStatus overwrites obj's .status via the status subresource and
// stamps UpdatedAt to now — obj must already carry the correct
// name/namespace/resourceVersion (i.e. it's whatever Create/Get just
// returned), since UpdateStatus, like Update, is a full-object call
// under the hood.
func setStatus(ctx context.Context, resource dynamic.ResourceInterface, obj *unstructured.Unstructured, status Status) error {
	status.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	statusMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&status)
	if err != nil {
		return fmt.Errorf("converting status for %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	obj.Object["status"] = statusMap

	if _, err := resource.UpdateStatus(ctx, obj, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating status for %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	return nil
}

// MarkReviewed advances a proposal from ApprovalDraft to
// ApprovalReviewed — called by `review` (cmd/landlock-genprof/review.go)
// on every successful run. A no-op, not an error, if the proposal is
// already past Draft (Reviewed/Approved/Rejected): looking at a
// proposal again should never silently undo an actual approve/reject
// decision, or even just repeatedly bump UpdatedAt for no reason.
//
// On update conflict, retries the entire get-check-update cycle against
// the fresh object to re-validate the current state and ensure the
// transition is still legal.
func MarkReviewed(ctx context.Context, client dynamic.Interface, namespace, name string) error {
	resource := client.Resource(securityProfileProposalGVR).Namespace(namespace)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		obj, err := resource.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("fetching SecurityProfileProposal %s/%s before marking reviewed: %w", namespace, name, err)
		}

		current, err := statusFromObject(obj)
		if err != nil {
			return err
		}
		if current.ApprovalState != ApprovalDraft {
			return nil
		}

		return setStatus(ctx, resource, obj, Status{ApprovalState: ApprovalReviewed})
	})
}

// SetApprovalState records an explicit human decision — always applies,
// unlike MarkReviewed, since an actual approve/reject call should always
// win regardless of the proposal's current state (including moving a
// Rejected proposal back to Approved after reconsidering, or the
// reverse).
//
// On update conflict, retries the entire get-update cycle against
// the fresh object to ensure the desired state is persisted.
func SetApprovalState(ctx context.Context, client dynamic.Interface, namespace, name string, state ApprovalState, reason string, expectedCandidateDigest string) error {
	resource := client.Resource(securityProfileProposalGVR).Namespace(namespace)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		obj, err := resource.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("securityprofileproposal %s/%s not found", namespace, name)
		}
		if err != nil {
			return fmt.Errorf("fetching SecurityProfileProposal %s/%s before setting approval state: %w", namespace, name, err)
		}
		currentStatus, err := statusFromObject(obj)
		if err != nil {
			return err
		}

		// If approving, require a matching expectedCandidateDigest from the
		// reviewer to protect against stale-reviewer misbinding. For other
		// states, clear any previously recorded approved digest.
		if state == ApprovalApproved {
			if expectedCandidateDigest == "" {
				return fmt.Errorf("approving requires expectedCandidateDigest")
			}

			// Read spec and compute current candidate digest
			specMap, found, err := unstructured.NestedMap(obj.Object, "spec")
			if err != nil {
				return fmt.Errorf("reading spec from SecurityProfileProposal %s/%s: %w", namespace, name, err)
			}
			if !found {
				return fmt.Errorf("securityprofileproposal %s/%s has no spec", namespace, name)
			}
			var spec Spec
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(specMap, &spec); err != nil {
				return fmt.Errorf("converting spec from SecurityProfileProposal %s/%s: %w", namespace, name, err)
			}

			version, err := normalizedCandidateVersion(spec.CandidateVersion)
			if err != nil {
				return err
			}
			var computed, reviewDigest string
			if version == CandidateVersionV2 {
				candidate, err := spec.candidateV2()
				if err != nil {
					return err
				}
				computed, err = CandidateDigestV2(candidate)
				if err != nil {
					return err
				}
				reviewContext, err := spec.reviewContextV2()
				if err != nil {
					return err
				}
				reviewDigest, err = ReviewContextDigestV2(reviewContext)
				if err != nil {
					return err
				}
			} else {
				computed, err = CandidateDigest(spec)
				if err != nil {
					return err
				}
			}

			// Validate expectedCandidateDigest formatting
			if err := ValidateCandidateDigest(expectedCandidateDigest); err != nil {
				return err
			}

			if expectedCandidateDigest != computed {
				return fmt.Errorf("expected candidate digest mismatch: provided %s, computed %s", expectedCandidateDigest, computed)
			}
			if version == CandidateVersionV2 && obj.GetUID() == "" {
				return fmt.Errorf("candidate-v2 approval requires Proposal UID-bound custody")
			}

			var snapshot *ApprovalSnapshot
			if uid := string(obj.GetUID()); uid != "" {
				snapshot = &ApprovalSnapshot{
					ProposalUID:              uid,
					ApprovalMechanismVersion: version,
					ApprovedCandidateDigest:  computed,
					ReviewContextDigest:      reviewDigest,
					ApprovedAt:               time.Now().UTC().Format(time.RFC3339Nano),
				}
			}
			status := Status{ApprovalState: state, Reason: reason, ApprovedCandidateDigest: computed, ApprovedReviewContextDigest: reviewDigest, ApprovalMechanismVersion: version, LastApprovalSnapshot: snapshot}
			return setStatus(ctx, resource, obj, status)
		}

		// Clearing digest on non-approved transitions is recommended
		// to avoid retaining active authorization material.
		return setStatus(ctx, resource, obj, Status{ApprovalState: state, Reason: reason, ApprovedCandidateDigest: "", ApprovedReviewContextDigest: "", LastApprovalSnapshot: currentStatus.LastApprovalSnapshot})
	})
}
