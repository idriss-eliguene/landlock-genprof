package applyproposal

import (
	"fmt"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/idriss-eliguene/landlock-genprof/internal/attempt"
)

// MutationSnapshot and DigestJSON are the narrow immutable-data helpers used
// by rollback when reconstructing and checking application custody.
func MutationSnapshot(gvk schema.GroupVersionKind, obj *unstructured.Unstructured) []byte {
	return mutationSnapshot(gvk, obj)
}

func DigestJSON(value []byte) string { return digestJSON(value) }

func HasSuccessfulMutation(records []attempt.MutationRecord) bool {
	return hasSuccessfulMutation(records)
}

// ValidateCompositionCompatibilityForSlugs preserves the apply composition
// guard for rollback's restored workload readiness check without exporting
// the apply planner representation.
func ValidateCompositionCompatibilityForSlugs(slugs []string) error {
	var podlock, seccomp bool
	for _, slug := range slugs {
		podlock = podlock || slug == "podlock"
		seccomp = seccomp || slug == "spo-seccompprofile"
	}
	if podlock && seccomp {
		return fmt.Errorf("PodLock + application-derived Seccomp composition is unsupported: runtime compatibility is unproven")
	}
	return nil
}
