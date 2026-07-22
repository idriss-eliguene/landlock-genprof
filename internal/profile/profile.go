// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package profile defines the Behavior IR (intermediate representation):
// the internal, technology-neutral description of what a workload was
// observed doing, independent of any specific output format (PodLock,
// Kubernetes NetworkPolicy, Cilium, ...).
//
// internal/policy builds a BehaviorProfile from raw tracer events;
// exporters (internal/exporter/...) read a BehaviorProfile and produce a
// format-specific artifact. The dependency only ever runs exporter -> IR,
// never the other way — this package must not import a specific output
// format, YAML, or Kubernetes/collector types. deps_test.go enforces this
// statically, since nothing at the Go type level would otherwise catch an
// accidental import the other way.
package profile

// BehaviorProfile is the full observed behavior of a workload during a
// training run. Process/syscall behavior beyond filesystem and network is
// a natural, additive extension point — not added yet since no collector
// produces it (see docs/roadmap.md).
type BehaviorProfile struct {
	Filesystem FilesystemProfile
	Network    NetworkProfile
}

// FilesystemProfile is the filesystem part of a BehaviorProfile: one
// FileAccess per distinct path, deduplicated and aggregated by
// internal/policy.Synthesize.
type FilesystemProfile struct {
	Accesses []FileAccess
}

// FileAccess records the set of permissions observed on a single path.
// Permissions is a set, not a single joint label (like PodLock's
// "readWriteExec"): collapsing read/write/execute into a
// technology-specific joint category is an exporter's job, not the IR's
// — see internal/exporter/podlock.
type FileAccess struct {
	Path        string
	Permissions []FilePermission
	Confidence  Confidence
	SeenCount   int
}

// HasPermission reports whether p was observed for this access.
func (fa FileAccess) HasPermission(p FilePermission) bool {
	for _, perm := range fa.Permissions {
		if perm == p {
			return true
		}
	}
	return false
}

// NetworkProfile is the network part of a BehaviorProfile: one
// NetworkAccess per distinct (port, direction) pair, aggregated by
// internal/policy.Synthesize.
type NetworkProfile struct {
	Accesses []NetworkAccess
}

// NetworkAccess records a TCP port observed either as a connect (egress)
// or bind (ingress) target — the only two rights the trace_tcpconnect/
// trace_bind gadgets and Landlock's own LANDLOCK_ACCESS_NET_* rights cover
// (see README's gadget table). No Protocol field: TCP is the only thing
// either gadget or Landlock's network rights represent today.
type NetworkAccess struct {
	Port       int
	Direction  NetworkDirection
	Confidence Confidence
	SeenCount  int
}

// NetworkDirection is which side of a TCP handshake a NetworkAccess was
// observed on.
type NetworkDirection string

const (
	DirectionEgress  NetworkDirection = "egress"  // connect()
	DirectionIngress NetworkDirection = "ingress" // bind()
)

// FilePermission is one elementary filesystem right observed on a path.
type FilePermission string

const (
	PermissionRead    FilePermission = "read"
	PermissionWrite   FilePermission = "write"
	PermissionExecute FilePermission = "execute"
)

// Confidence indicates how certain a generated access is, based on how
// many training runs it was observed in.
type Confidence string

const (
	ConfidenceLow    Confidence = "low"    // seen only once
	ConfidenceMedium Confidence = "medium" // seen across multiple runs, inconsistently
	ConfidenceHigh   Confidence = "high"   // seen consistently
)
