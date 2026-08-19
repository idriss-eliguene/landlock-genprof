package adapter

import (
	"fmt"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation"
	"github.com/idriss-eliguene/landlock-genprof/internal/semantic"
)

// projectFilesystem validates filesystem observation payload and constructs
// a FileAccess semantic.Proposition. Pure function depending only on obs.
func projectFilesystem(obs observation.Observation) (semantic.Proposition, error) {
	allowed := map[string]struct{}{"read": {}, "write": {}, "read_write": {}, "exec": {}}
	if obs.Path == "" {
		return semantic.Proposition{}, fmt.Errorf("empty path (mode=%q)", obs.Mode)
	}
	if _, ok := allowed[obs.Mode]; !ok {
		return semantic.Proposition{}, fmt.Errorf("unsupported mode=%q", obs.Mode)
	}
	argsRecord := map[string]semantic.SnapshotValue{}
	if obs.Path != "" {
		argsRecord["path"] = semantic.NewLiteral("string", obs.Path)
	}
	if obs.Mode != "" {
		argsRecord["mode"] = semantic.NewLiteral("string", obs.Mode)
	}
	if obs.IsDir {
		argsRecord["isDir"] = semantic.NewLiteral("bool", "true")
	}
	if obs.Truncate {
		argsRecord["truncate"] = semantic.NewLiteral("bool", "true")
	}
	rec := semantic.NewRecord(argsRecord)
	prop := semantic.NewProposition(semantic.NewIdentityRef("Phase", "p"), semantic.Actual, semantic.NewIdentityRef("Term", "FileAccess"), []semantic.SnapshotValue{rec}, semantic.NewValidTime(nil, nil), semantic.QuantThisInstance)
	return prop, nil
}
