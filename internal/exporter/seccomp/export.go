// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package seccomp converts a Behavior IR (internal/profile) into a
// seccomp profile (pkg/seccomp) and serializes it to JSON.
//
// This is a sibling of internal/exporter/podlock and
// internal/exporter/networkpolicy, not a variant of either: unlike those
// two, its output must stay plain, comment-free JSON — a seccomp profile
// is loaded directly by the kubelet/container runtime from a file, never
// `kubectl apply`d — so it can't carry a `# confidence: ...` YAML comment
// the way the other two exporters do. See ToJSON.
package seccomp

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
	"github.com/idriss-eliguene/landlock-genprof/pkg/seccomp"
)

// defaultAction denies anything not explicitly allowed — the standard,
// conservative seccomp default (matches advise_seccomp's own output, see
// internal/tracer/trace_linux.go's runSeccompTracer).
const defaultAction = "SCMP_ACT_ERRNO"

// allowAction is the action every observed syscall is granted: this
// exporter only ever produces one rule bucket (all observed syscalls,
// allowed) — splitting by Confidence into separate buckets isn't
// meaningful for seccomp the way it might look tempting: a single denied
// syscall breaks the container outright, so there's no partial-trust
// action to fall back to. Low-confidence syscalls are surfaced for human
// review instead (see cmd/landlock-genprof/trace.go's
// writeSeccompProfile), not silently excluded or bucketed differently.
const allowAction = "SCMP_ACT_ALLOW"

// ToProfile converts a BehaviorProfile's syscall observations into a
// seccomp profile ready to be serialized.
func ToProfile(syscalls profile.SyscallProfile) *seccomp.Profile {
	names := make([]string, len(syscalls.Accesses))
	for i, access := range syscalls.Accesses {
		names[i] = access.Name
	}
	sort.Strings(names)

	var rules []seccomp.SyscallRule
	if len(names) > 0 {
		rules = []seccomp.SyscallRule{{Names: names, Action: allowAction}}
	}

	return &seccomp.Profile{
		DefaultAction: defaultAction,
		Architectures: syscalls.Architectures,
		Syscalls:      rules,
	}
}

// ToJSON serializes a seccomp profile to indented JSON, as written to
// <pod>-seccomp.json by the CLI (see cmd/landlock-genprof).
//
// Unlike internal/exporter/podlock.ToYAML and
// internal/exporter/networkpolicy.ToYAML, this does not re-parse through
// gopkg.in/yaml.v3 to attach a `# confidence: ...` comment: comments
// aren't legal JSON, and this file must stay loadable as-is by the
// kubelet/container runtime — see the package doc. Confidence is instead
// printed to stdout by the CLI (writeSeccompProfile), not embedded here.
func ToJSON(p *seccomp.Profile) ([]byte, error) {
	out, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling seccomp profile: %w", err)
	}
	return append(out, '\n'), nil
}
