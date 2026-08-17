// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

// Package tracer captures a pod's syscall events during a training run,
// building on the existing Inspektor Gadget gadgets (trace_open,
// trace_tcp, trace_bind, trace_exec) rather than writing eBPF
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

import (
	"strings"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation"
)

// Event represents an access observed during the training run, before
// translation into Landlock rights.
// ProvenanceDescriptor is an acquisition-local descriptor describing
// which backend produced an event. It is intentionally lightweight and
// lives in internal/tracer so the acquisition layer — which knows the
// collector/gadget context — declares provenance. Keep fields as
// strings so this package does not need to import internal/evidence.
type ProvenanceDescriptor struct {
	BackendKind      string
	OriginType       string
	BackendAgentID   string
	CollectorVersion string
}

// TimestampExtractionDiagnostic captures why timestamp_raw decoding failed for
// a single emitted event. It is runtime-only diagnostic metadata and must not
// affect semantic evaluation.
type TimestampExtractionDiagnostic struct {
	DataSource    string
	Field         string
	AccessorError string
	RawLen        int
	RawHex        string
}

// Event represents an access observed during the training run, before
// translation into Landlock rights.
type Event struct {
	Timestamp time.Time
	Syscall   string // e.g. "openat", "connect", "bind", "execve", or a bare syscall name when Mode == "syscall"
	Path      string // file path involved, if applicable
	Port      int    // network port involved, if applicable
	Mode      string // "read", "write", "read_write", "exec", "egress", "ingress", "syscall"
	// IsDir is true when Path itself was opened as a directory (e.g. `ls
	// <dir>` opens <dir> with O_DIRECTORY to list it), as opposed to a
	// regular file. Synthesize() needs this: aggregating by the
	// *parent* of an opened path is correct for a file, but wrong for a
	// directory — /etc opened directly is not "some file under /".
	// Found from a real training run producing a `readOnly: [/]` rule
	// (see docs/policy-synthesis.md).
	IsDir bool
	// Truncate is true when an openat(2) included O_TRUNC — Landlock's
	// own TRUNCATE right (ABI3), independent of the read/write access
	// mode: open(path, O_WRONLY|O_TRUNC) is both a write and a truncate.
	// Read from the same raw flags value trace_open already reports for
	// Mode (see trace_linux.go's modeFromOpenFlags/runOpenTracer) — no
	// new gadget or syscall hook needed, just one more bit of data
	// already flowing through the pipeline.
	Truncate bool

	// Provenance is a runtime-only, acquisition-declared descriptor. It
	// MUST NOT be serialized by tracer consumers directly — evidence
	// emission translates this into the document-local ProvenanceID.
	// The json:"-" tag ensures it is not marshaled accidentally.
	Provenance *ProvenanceDescriptor `json:"-"`
	// TimestampDiag carries extractor-level context when Timestamp could not be
	// decoded from timestamp_raw.
	TimestampDiag *TimestampExtractionDiagnostic `json:"-"`
}

// IsFilesystemEvent reports whether ev should be treated as a filesystem
// observation. This predicate is derived solely from the tracer.Event
// contract: it does not reference policy or semantic packages. It is the
// single authoritative classifier callers should use to partition a
// mixed event stream before directing events to a filesystem-only
// consumer.
func IsFilesystemEvent(ev Event) bool {
	obs := ToObservation(ev)
	return obs.Kind == observation.KindFilesystem
}

// ToObservation normalizes a tracer.Event into an observation.Observation
// preserving all raw fields exactly and assigning a Kind based on the
// tracer contract. This function is the canonical place for event-kind
// classification.
func ToObservation(ev Event) observation.Observation {
	kind := observation.KindOther
	// Filesystem: absolute path and one of the filesystem modes
	if ev.Path != "" && strings.HasPrefix(ev.Path, "/") {
		switch ev.Mode {
		case "read", "write", "read_write", "exec":
			kind = observation.KindFilesystem
		}
	}
	// Capability and syscall modes are evaluated before the network
	// Syscall-name check below. advise_seccomp emits advisory events
	// with Mode == "syscall" whose Syscall field is a bare syscall name
	// that can itself be a network syscall (bind, connect, sendmsg,
	// recvmsg, ...). Checking Mode == "syscall" first ensures these are
	// classified as KindSyscall, not misclassified as KindNetwork purely
	// because of the syscall's name. See diagnostic run 32028412953:
	// a zero-timestamp KindNetwork advisory event (Syscall: "bind",
	// Mode: "syscall") was fatally rejected by the adapter, when it
	// should have been classified KindSyscall and accepted as a
	// by-design zero-timestamp advisory event.
	if ev.Mode == "capability" {
		if kind == observation.KindOther {
			kind = observation.KindCapability
		}
	} else if ev.Mode == "syscall" {
		if kind == observation.KindOther {
			kind = observation.KindSyscall
		}
	}
	// Network: connect/bind or modes egress/ingress
	switch ev.Syscall {
	case "connect", "bind":
		if kind == observation.KindOther {
			kind = observation.KindNetwork
		}
	}
	if ev.Mode == "egress" || ev.Mode == "ingress" {
		if kind == observation.KindOther {
			kind = observation.KindNetwork
		}
	}

	return observation.Observation{
		Kind:      kind,
		Path:      ev.Path,
		Mode:      ev.Mode,
		Syscall:   ev.Syscall,
		IsDir:     ev.IsDir,
		Truncate:  ev.Truncate,
		Port:      ev.Port,
		Timestamp: ev.Timestamp,
	}
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
