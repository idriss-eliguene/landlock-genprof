// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Enforcement readiness for the governed apply path — see
// docs/adr/0007-governed-apply-ordering-and-enforcement-readiness.md.
//
// The problem this file exists for: applying a resource is not the same
// as that resource being usable. An SPO SeccompProfile only becomes
// enforceable once SPO's daemon has materialized it onto the node at
// operator/<ns>/<name>.json; until then a workload whose
// securityContext.seccompProfile.localhostProfile points at that path
// cannot start at all — containerd refuses a profile that doesn't
// resolve to a real file (see internal/exporter/spo.LocalhostProfilePath's
// own doc comment, and the CrashLoopBackOff recorded in
// demo/golden/workload.yaml).
//
// So the binding artifact waits. Not for a fixed duration, and not for
// "the resource exists" — for the backend reporting that *the approved
// content* is ready.

package main

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// applyClass orders the plan by dependency rather than by declaration
// order. ADR-0007's ordering is a property of what each artifact *is*,
// not of where it happens to sit in a slice literal, so it's expressed
// here rather than by rearranging proposalArtifacts (which also drives
// the review display, a presentation concern with no safety content).
type applyClass int

const (
	// classEnforcement is an artifact an external controller reconciles
	// and that the binding artifact may reference by name or path.
	classEnforcement applyClass = iota
	// classLivePolicy is a resource that takes effect on its own and is
	// referenced by nothing. Ordered before the binding artifact so a
	// recreated pod is covered from the moment it starts — a preference,
	// not a dependency.
	classLivePolicy
	// classBinding mutates the workload itself. Always last: it is the
	// only artifact that is both destructive and workload-affecting.
	classBinding
)

func applyClassFor(slug string) applyClass {
	switch slug {
	case "spo-seccompprofile", "podlock":
		return classEnforcement
	case patchedManifestSlug:
		return classBinding
	default:
		// networkpolicy, and anything added later that isn't explicitly
		// an enforcement or binding artifact.
		return classLivePolicy
	}
}

// seccompProfileGVR is SPO's own resource, matching the apiVersion
// internal/exporter/spo already generates.
var seccompProfileGVR = schema.GroupVersionResource{
	Group:    "security-profiles-operator.x-k8s.io",
	Version:  "v1beta1",
	Resource: "seccompprofiles",
}

// readinessPollInterval is how often the gate re-reads backend state.
// A package-level var so tests don't have to spend real seconds proving
// a negative — same seam style as applyManifest and the dynamic-client
// constructors above.
var readinessPollInterval = 2 * time.Second

// afterEnforcementReady is a test seam invoked once the readiness gate
// has passed and before the pre-binding authority gate (Gate 3), so a
// test can revoke approval in exactly the window a long wait opens.
var afterEnforcementReady func()

// seccompRequirement is one thing the binding artifact needs before it
// can be applied: a SeccompProfile, at a specific namespace/name, whose
// materialized path matches what the workload will reference, carrying
// the enforcement content that was approved.
type seccompRequirement struct {
	namespace string
	name      string
	// localhostProfile is the exact value the binding artifact carries in
	// securityContext.seccompProfile.localhostProfile.
	localhostProfile string
	// wantSpec is the approved enforcement content, or nil when the
	// proposal carries no SeccompProfile artifact for this reference.
	wantSpec map[string]interface{}
}

// enforcementRequirements derives what must be ready from what the
// binding artifact actually references — ADR-0007 INV-APPLY-ORDER-01 is
// stated in terms of "every enforcement artifact it references", so the
// requirement is read off the binding artifact rather than inferred from
// which artifacts this particular run happens to be applying.
//
// That distinction matters: --skip can exclude the SeccompProfile from
// this run while the approved patched manifest still points at it. The
// profile may legitimately already exist and be ready from an earlier
// apply — in which case binding is safe — or it may not exist at all, in
// which case binding produces a pod that cannot start. Only checking the
// live backend can tell those apart.
func enforcementRequirements(plan []plannedArtifact, approvedSeccomp *unstructured.Unstructured) []seccompRequirement {
	var binding *plannedArtifact
	for i := range plan {
		if plan[i].slug == patchedManifestSlug {
			binding = &plan[i]
			break
		}
	}
	if binding == nil {
		return nil
	}

	var reqs []seccompRequirement
	seen := make(map[string]bool)
	for _, path := range referencedLocalhostProfiles(binding.obj) {
		if seen[path] {
			continue
		}
		seen[path] = true

		ns, name, ok := parseLocalhostProfilePath(path)
		if !ok {
			// A localhostProfile this tool didn't generate (not
			// operator/<ns>/<name>.json). Nothing to check it against —
			// record it so the gate can refuse rather than pretend.
			reqs = append(reqs, seccompRequirement{localhostProfile: path})
			continue
		}

		req := seccompRequirement{namespace: ns, name: name, localhostProfile: path}
		if approvedSeccomp != nil &&
			approvedSeccomp.GetNamespace() == ns && approvedSeccomp.GetName() == name {
			req.wantSpec = enforcementSpec(approvedSeccomp)
		}
		reqs = append(reqs, req)
	}
	return reqs
}

// referencedLocalhostProfiles collects every localhostProfile the object's
// containers reference, from either a bare pod spec or a workload
// template's pod spec.
func referencedLocalhostProfiles(obj *unstructured.Unstructured) []string {
	if obj == nil {
		return nil
	}

	podSpecPaths := [][]string{
		{"spec"},                     // Pod
		{"spec", "template", "spec"}, // Deployment / StatefulSet / DaemonSet
	}

	var out []string
	for _, base := range podSpecPaths {
		for _, field := range []string{"containers", "initContainers"} {
			items, found, err := unstructured.NestedSlice(obj.Object, append(append([]string{}, base...), field)...)
			if err != nil || !found {
				continue
			}
			for _, item := range items {
				container, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				profile, found, err := unstructured.NestedString(container,
					"securityContext", "seccompProfile", "localhostProfile")
				if err == nil && found && profile != "" {
					out = append(out, profile)
				}
			}
		}
	}
	return out
}

// parseLocalhostProfilePath reverses internal/exporter/spo.LocalhostProfilePath's
// "operator/<namespace>/<name>.json" convention.
func parseLocalhostProfilePath(path string) (namespace, name string, ok bool) {
	parts := strings.Split(path, "/")
	if len(parts) != 3 || parts[0] != "operator" || !strings.HasSuffix(parts[2], ".json") {
		return "", "", false
	}
	namespace = parts[1]
	name = strings.TrimSuffix(parts[2], ".json")
	if namespace == "" || name == "" {
		return "", "", false
	}
	return namespace, name, true
}

// enforcementSpec extracts only the fields that decide what a seccomp
// profile actually enforces. Deliberately not the whole object: SPO
// writes status and may add its own metadata, and comparing those would
// make the identity check fail for reasons that have nothing to do with
// enforcement.
func enforcementSpec(obj *unstructured.Unstructured) map[string]interface{} {
	if obj == nil {
		return nil
	}
	out := make(map[string]interface{}, 3)
	for _, field := range []string{"defaultAction", "architectures", "syscalls"} {
		if value, found, err := unstructured.NestedFieldNoCopy(obj.Object, "spec", field); err == nil && found {
			out[field] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// waitForEnforcementReady blocks until every requirement is ready, or
// fails closed. Ready means all of: the resource exists, SPO reports it
// materialized at exactly the path the workload will reference, and its
// live enforcement content still equals what was approved.
//
// Failing closed is the whole point — every return path other than nil
// means the caller must not apply the binding artifact.
func waitForEnforcementReady(ctx context.Context, stdout io.Writer, client dynamic.Interface, reqs []seccompRequirement, timeout time.Duration) error {
	if len(reqs) == 0 {
		return nil
	}

	for _, req := range reqs {
		if req.name == "" {
			return &exitCodeError{code: 2, wrapped: fmt.Errorf(
				"the workload references seccomp profile %q, which this tool cannot resolve to a SeccompProfile "+
					"(expected operator/<namespace>/<name>.json); refusing to bind the workload to a profile whose readiness cannot be established",
				req.localhostProfile)}
		}

		fmt.Fprintf(stdout, "waiting for SPO to reconcile SeccompProfile %s/%s (up to %s)\n",
			req.namespace, req.name, timeout)

		if err := waitForSeccompProfileReady(ctx, client, req, timeout); err != nil {
			return err
		}

		fmt.Fprintf(stdout, "ready: SeccompProfile %s/%s -> %s\n",
			req.namespace, req.name, req.localhostProfile)
	}
	return nil
}

func waitForSeccompProfileReady(ctx context.Context, client dynamic.Interface, req seccompRequirement, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resource := client.Resource(seccompProfileGVR).Namespace(req.namespace)
	var lastReason string

	for {
		obj, err := resource.Get(waitCtx, req.name, metav1.GetOptions{})
		switch {
		case err == nil:
			ready, reason, fatal := seccompProfileReady(obj, req)
			if ready {
				return nil
			}
			if fatal != nil {
				return &exitCodeError{code: 2, wrapped: fatal}
			}
			lastReason = reason
		case seccompAPIUnavailable(err):
			// The resource type itself isn't served. Checked before the
			// generic NotFound below, which it would otherwise be
			// swallowed by — see seccompAPIUnavailable's doc comment.
			return &exitCodeError{code: 2, wrapped: fmt.Errorf(
				"the SeccompProfile API is not available on this cluster, so the readiness of %s/%s "+
					"cannot be established (is security-profiles-operator installed?); "+
					"workload binding not applied: %w", req.namespace, req.name, err)}
		case apierrors.IsNotFound(err):
			// The object doesn't exist yet. Normal while a controller is
			// still catching up, so keep waiting.
			lastReason = "SeccompProfile does not exist yet"
		case fatalAPIError(err):
			return &exitCodeError{code: 2, wrapped: fmt.Errorf(
				"cannot establish readiness of SeccompProfile %s/%s: %w", req.namespace, req.name, err)}
		default:
			// Genuinely transient: timeouts, throttling, a server that is
			// briefly unavailable. Worth retrying inside the budget.
			lastReason = fmt.Sprintf("reading SeccompProfile failed: %v", err)
		}

		select {
		case <-waitCtx.Done():
			return &exitCodeError{code: 2, wrapped: fmt.Errorf(
				"timed out after %s waiting for SeccompProfile %s/%s to become ready (%s); "+
					"workload binding not applied", timeout, req.namespace, req.name, lastReason)}
		case <-time.After(readinessPollInterval):
		}
	}
}

// seccompProfileReady reports whether obj is ready for req. A fatal
// error means waiting longer cannot help: the backend has materialized
// something, but not what was approved.
func seccompProfileReady(obj *unstructured.Unstructured, req seccompRequirement) (ready bool, reason string, fatal error) {
	// Identity before readiness: a backend reporting "ready" for content
	// that isn't the approved content is not ready for our purposes
	// (ADR-0007 INV-APPLY-ORDER-02). Checked first so drift is reported
	// as drift rather than as a timeout.
	if req.wantSpec != nil {
		got := enforcementSpec(obj)
		if !reflect.DeepEqual(req.wantSpec, got) {
			return false, "", fmt.Errorf(
				"SeccompProfile %s/%s no longer carries the approved enforcement content "+
					"(defaultAction/architectures/syscalls changed since it was applied); "+
					"workload binding not applied", req.namespace, req.name)
		}
	}

	installed, found, err := unstructured.NestedString(obj.Object, "status", "localhostProfile")
	if err != nil {
		return false, fmt.Sprintf("reading status.localhostProfile: %v", err), nil
	}
	if !found || installed == "" {
		return false, "status.localhostProfile not populated yet", nil
	}
	if installed != req.localhostProfile {
		// Not a transient state: SPO has decided where this profile
		// lives, and it isn't where the workload will look.
		return false, "", fmt.Errorf(
			"SeccompProfile %s/%s materialized at %q but the workload references %q; "+
				"workload binding not applied", req.namespace, req.name, installed, req.localhostProfile)
	}
	return true, "", nil
}

// seccompAPIUnavailable reports whether err means the SeccompProfile
// resource type itself is not served — SPO not installed — as opposed to
// the object simply not existing yet.
//
// Both surface as HTTP 404 with reason NotFound, so apierrors.IsNotFound
// cannot separate them and checking it first would swallow this case
// into the retry path, burning the whole readiness budget before failing
// with a message that sends the reader looking for a missing object
// rather than a missing operator.
//
// The distinction is structural rather than textual. Confirmed against a
// live cluster: a missing *object* carries Status.Details with
// Group/Kind/Name populated ("securityprofileproposals.landlockgenprof.io
// \"x\" not found"), while a missing *resource type* returns a bare
// "the server could not find the requested resource" with Details empty.
func seccompAPIUnavailable(err error) bool {
	if !apierrors.IsNotFound(err) {
		return false
	}
	status, ok := err.(apierrors.APIStatus)
	if !ok {
		return false
	}
	details := status.Status().Details
	return details == nil || details.Name == ""
}

// fatalAPIError reports whether err is one that waiting cannot fix.
// Credentials, permissions and malformed requests do not resolve
// themselves inside a readiness budget, so consuming the budget on them
// only delays a failure that is already certain.
func fatalAPIError(err error) bool {
	return apierrors.IsUnauthorized(err) ||
		apierrors.IsForbidden(err) ||
		apierrors.IsInvalid(err) ||
		apierrors.IsBadRequest(err) ||
		apierrors.IsMethodNotSupported(err)
}
