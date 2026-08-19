package adapter

import (
	"fmt"
	"strings"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation"
	"github.com/idriss-eliguene/landlock-genprof/internal/semantic"
)

// projectCapability validates capability observation payload and constructs
// a CapabilityCheckObserved Proposition.
func projectCapability(obs observation.Observation) (semantic.Proposition, error) {
	if obs.Mode != "capability" {
		return semantic.Proposition{}, fmt.Errorf("expected mode=\"capability\", got %q", obs.Mode)
	}
	name := strings.TrimSpace(obs.Syscall)
	if name == "" {
		return semantic.Proposition{}, fmt.Errorf("empty capability name")
	}
	args := map[string]semantic.SnapshotValue{"name": semantic.NewLiteral("string", name)}
	rec := semantic.NewRecord(args)
	prop := semantic.NewProposition(semantic.NewIdentityRef("Phase", "p"), semantic.Actual, semantic.NewIdentityRef("Term", "CapabilityCheckObserved"), []semantic.SnapshotValue{rec}, semantic.NewValidTime(nil, nil), semantic.QuantThisInstance)
	return prop, nil
}
