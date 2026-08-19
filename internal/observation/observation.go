// Package observation provides a minimal, policy-neutral normalized
// runtime observation value derived from tracer.Event. It exists purely
// for engineering reuse by downstream consumers (policy, semantic
// adapter) and intentionally carries no policy- or semantic-specific
// types.
package observation

import "time"

// Kind classifies the observation into a small engineering set of kinds.
// No RFC-level semantics are attached to these values.
type Kind uint8

const (
	KindOther Kind = iota
	KindFilesystem
	KindNetwork
	KindSyscall
	KindCapability
)

// Observation is a small, dependency-light value type representing a
// single runtime observation. Fields mirror tracer.Event's exported
// fields and are preserved exactly; classification is additional
// metadata.
type Observation struct {
	Kind      Kind
	Path      string
	Mode      string
	Syscall   string
	IsDir     bool
	Truncate  bool
	Port      int
	Timestamp time.Time
}
