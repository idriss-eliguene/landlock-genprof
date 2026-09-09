package rollback

import (
	"errors"
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/attempt"
)

func TestInverseFailureClassificationIsStable(t *testing.T) {
	if got := inverseFailureResult(preDispatchFailure(errors.New("before"))); got != attempt.ResultFailed {
		t.Fatalf("pre-dispatch result = %q, want %q", got, attempt.ResultFailed)
	}
	if got := inverseFailureResult(postDispatchFailure(errors.New("after"))); got != attempt.ResultUnknown {
		t.Fatalf("post-dispatch result = %q, want %q", got, attempt.ResultUnknown)
	}
}

func TestInverseOperationPreservesExistingMapping(t *testing.T) {
	if got := inverseOperation(attempt.MutationRecord{Operation: "CREATE"}); got != "DELETE" {
		t.Fatalf("CREATE inverse = %q, want DELETE", got)
	}
	if got := inverseOperation(attempt.MutationRecord{Operation: "UPDATE"}); got != "UPDATE" {
		t.Fatalf("UPDATE inverse = %q, want UPDATE", got)
	}
}
