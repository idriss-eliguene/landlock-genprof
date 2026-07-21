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
}

// Options configures a training run.
type Options struct {
	PodName   string
	Namespace string
	Container string
	Duration  time.Duration
}

// Trace starts the capture and returns the observed events.
//
// TODO(M1, Student A): implement using the Inspektor Gadget gadgets
// (trace_open / trace_tcpconnect / trace_bind). Do not write an eBPF
// program from scratch for v1.
func Trace(opts Options) ([]Event, error) {
	panic("not implemented")
}
