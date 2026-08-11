package adapter

import (
	"fmt"
	"strings"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation"
	"github.com/idriss-eliguene/landlock-genprof/internal/semantic"
)

// projectSyscall validates syscall-profile observation payload and constructs
// a SyscallProfileAllowed Proposition.
func projectSyscall(obs observation.Observation) (semantic.Proposition, error) {
	if obs.Mode != "syscall" {
		return semantic.Proposition{}, fmt.Errorf("expected mode=\"syscall\", got %q", obs.Mode)
	}
	name := strings.TrimSpace(obs.Syscall)
	if name == "" {
		return semantic.Proposition{}, fmt.Errorf("empty syscall name")
	}
	args := map[string]semantic.SnapshotValue{"name": semantic.NewLiteral("string", name)}
	rec := semantic.NewRecord(args)
	prop := semantic.NewProposition(semantic.NewIdentityRef("Phase", "p"), semantic.Actual, semantic.NewIdentityRef("Term", "SyscallProfileAllowed"), []semantic.SnapshotValue{rec}, semantic.NewValidTime(nil, nil), semantic.QuantThisInstance)
	return prop, nil
}
