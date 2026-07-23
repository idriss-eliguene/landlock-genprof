// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package tracer captures a pod's syscall events during a training run,
// building on the existing Inspektor Gadget gadgets (trace_open,
// trace_tcpconnect, trace_bind, trace_exec) rather than writing eBPF
// programs from scratch.
//
// Architecture decision (see docs/roadmap.md): we consume the output of
// gadgets already maintained and tested by the CNCF community, which
// greatly reduces the project's risk of failure while keeping the
// differentiation on the syscall → Landlock rights mapping and on policy
// synthesis, both of which remain novel.
//
// Trace() itself is split by build tag (trace_linux.go / trace_other.go):
// the Inspektor Gadget Go SDK transitively pulls in Linux-only syscall
// code (eBPF, cgroups, ...) that simply doesn't compile on macOS/Windows.
// Keeping Event/Options here, with no such import, means internal/policy
// (which only needs the Event data shape) and anything built on top of it
// keep building on any OS — only the real capture implementation is
// Linux-gated, which matches reality: Landlock and eBPF only exist there.
package tracer

import "time"

// Event represents an access observed during the training run, before
// translation into Landlock rights.
type Event struct {
	Timestamp time.Time
	Syscall   string // e.g. "openat", "connect", "bind", "execve"
	Path      string // file path involved, if applicable
	Port      int    // network port involved, if applicable
	Mode      string // "read", "write", "read_write", "exec"
	// IsDir is true when Path itself was opened as a directory (e.g. `ls
	// <dir>` opens <dir> with O_DIRECTORY to list it), as opposed to a
	// regular file. Synthesize() needs this: aggregating by the
	// *parent* of an opened path is correct for a file, but wrong for a
	// directory — /etc opened directly is not "some file under /".
	// Found from a real training run producing a `readOnly: [/]` rule
	// (see docs/policy-synthesis.md).
	IsDir bool
}

// Options configures a training run.
type Options struct {
	PodName   string
	Namespace string
	Container string
	Duration  time.Duration
	// Binary is the observed entry point's path, e.g. /usr/sbin/nginx —
	// the same value the CLI takes as --binary. Used for two things: an
	// export-time label (internal/exporter/podlock), and — since this
	// field was added — to scope capture to processes whose comm matches
	// this binary's basename, so that e.g. a `kubectl exec ... -- ls`
	// during the training window isn't attributed to the traced binary.
	// See commFromBinaryPath in trace_linux.go and docs/e2e-demo.md
	// Finding 1.
	Binary string
	// Selector, if non-empty, scopes capture via a Kubernetes label
	// selector (operator.KubeManager.selector) instead of PodName — takes
	// priority over PodName when set. Used when the traced identity is a
	// workload (Deployment/DaemonSet) whose pod names change across
	// restarts, so a fixed PodName can't be pre-targeted the way a bare
	// pod or StatefulSet can (see internal/k8s.KeepsStableName) —
	// cmd/landlock-genprof/trace.go's traceWithRestart sets this instead
	// of PodName for those two owner kinds. Confirmed present in the
	// vendored SDK
	// (pkg/operators/common/container-selector.go's ParamSelector),
	// same confidence level as the already-proven podname/namespace/
	// containername params, not a guess.
	Selector string
}
