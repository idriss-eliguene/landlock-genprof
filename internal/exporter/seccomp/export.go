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

// runtimeBaselineSyscalls are syscalls the container runtime itself
// (runc, via containerd) needs to perform during container init — after
// installing the seccomp filter but before exec'ing into the traced
// binary — so a trace of the binary's own behavior never observes them.
//
// capget: confirmed live (2026-07-30) — a landlock-genprof-generated
// SeccompProfile applied to a real kind/SPO v0.7.1 cluster without it
// put the target pod in CrashLoopBackOff with "OCI runtime create
// failed: ... unable to get capability version from the kernel:
// operation not permitted", runc's own error (libcontainer/capabilities)
// when its capget(2) probe of the kernel's supported capability version
// is denied by the seccomp filter. Every profile this package has ever
// produced was missing it.
//
// futex: confirmed live (2026-07-30), same cluster, right after fixing
// capget — the container now got created but crashed immediately after
// ("cannot start a stopped process"), `kubectl logs --previous` showing
// a Go runtime panic ("The futex facility returned an unexpected error
// code") inside runc's own libcontainer.setupUser/finalizeNamespace
// (syscall.Setgid, called via cgo, standard_init_linux.go). runc's own
// init process — which applies the seccomp filter to itself before
// exec'ing into the traced binary — is written in Go, and the Go
// runtime's scheduler/GC depend on futex(2) throughout its own
// lifetime, not just at one identifiable startup step; nginx (an
// event-loop C program) never calls it, so no trace of nginx's own
// behavior would ever surface it.
//
// chdir: confirmed live (2026-07-30), same cluster, right after fixing
// futex — "OCI runtime create failed: ... chdir to cwd (\"/\") set in
// config.json failed: operation not permitted". runc's own
// finalizeNamespace (init_linux.go) calls it right after setupUser to
// set the container's configured working directory, before exec —
// nginx's own code never calls chdir again once running, so no trace of
// it ever surfaces this either.
//
// capset: NOT yet independently confirmed by its own crash — included
// as a predicted fix, not a "confirmed live" one, to save a round-trip:
// runc's finalizeNamespace calls capget, then setupUser (setgid/futex),
// then chdir, then ApplyCaps (capset) next, in that exact order — the
// same function that already produced two confirmed misses in a row at
// each of its first three steps. Every patched manifest this project
// generates sets an explicit securityContext.capabilities
// (internal/exporter/capabilities), which is exactly what ApplyCaps
// exists to enforce, making it very likely runc needs capset here too.
// If a live test after this fix still crash-loops with a *different*
// error, that's a live signal this prediction was wrong — don't assume
// capset silently fixed it without checking.
var runtimeBaselineSyscalls = []string{"capget", "capset", "chdir", "futex"}

// ToProfile converts a BehaviorProfile's syscall observations into a
// seccomp profile ready to be serialized.
func ToProfile(syscalls profile.SyscallProfile) *seccomp.Profile {
	seen := make(map[string]bool, len(syscalls.Accesses)+len(runtimeBaselineSyscalls))
	var names []string
	for _, access := range syscalls.Accesses {
		if !seen[access.Name] {
			seen[access.Name] = true
			names = append(names, access.Name)
		}
	}
	if len(names) > 0 {
		for _, name := range runtimeBaselineSyscalls {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
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
