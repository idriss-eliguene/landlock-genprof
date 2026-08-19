package adapter

import (
	"fmt"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation"
	"github.com/idriss-eliguene/landlock-genprof/internal/semantic"
)

// projectNetwork validates network observation payload and constructs a
// NetworkAccess semantic.Proposition. Pure function depending only on obs.
func projectNetwork(obs observation.Observation) (semantic.Proposition, error) {
	if obs.Port == 0 {
		return semantic.Proposition{}, fmt.Errorf("missing port")
	}
	var dirFromSyscall, dirFromMode string
	if obs.Syscall == "bind" {
		dirFromSyscall = "ingress"
	} else if obs.Syscall == "connect" {
		dirFromSyscall = "egress"
	}
	if obs.Mode == "ingress" {
		dirFromMode = "ingress"
	} else if obs.Mode == "egress" {
		dirFromMode = "egress"
	}
	if dirFromSyscall != "" && dirFromMode != "" && dirFromSyscall != dirFromMode {
		return semantic.Proposition{}, fmt.Errorf("conflicting syscall/mode (syscall=%q mode=%q)", obs.Syscall, obs.Mode)
	}
	direction := "egress"
	if dirFromSyscall != "" {
		direction = dirFromSyscall
	} else if dirFromMode != "" {
		direction = dirFromMode
	}
	if direction == "" {
		return semantic.Proposition{}, fmt.Errorf("unsupported network mode/syscall (mode=%q syscall=%q)", obs.Mode, obs.Syscall)
	}
	argsRecord := map[string]semantic.SnapshotValue{}
	argsRecord["port"] = semantic.NewLiteral("int", fmt.Sprintf("%d", obs.Port))
	argsRecord["direction"] = semantic.NewLiteral("string", direction)
	rec := semantic.NewRecord(argsRecord)
	prop := semantic.NewProposition(semantic.NewIdentityRef("Phase", "p"), semantic.Actual, semantic.NewIdentityRef("Term", "NetworkAccess"), []semantic.SnapshotValue{rec}, semantic.NewValidTime(nil, nil), semantic.QuantThisInstance)
	return prop, nil
}
