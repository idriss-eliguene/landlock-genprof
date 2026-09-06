// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

//go:build !linux

package tracer

import (
	"context"
	"fmt"
)

// Trace is not available on non-Linux platforms: Landlock and eBPF, and
// therefore Inspektor Gadget's gadgets, are Linux-only. Build and test
// landlock-genprof from the dev VM (see HOW_TO_START.md) for anything
// touching the tracer — see trace_linux.go for the real implementation.
func Trace(opts Options, onReady func()) ([]Event, []string, error) {
	return nil, nil, fmt.Errorf("tracer.Trace: not supported on this platform (Landlock/eBPF are Linux-only)")
}

func TraceFilesystemSourceWithIdentity(ctx context.Context, opts Options, onAttached func(error), emit func(Event, RuntimeIdentity)) error {
	return fmt.Errorf("tracer.TraceFilesystemSourceWithIdentity: not supported on this platform (Landlock/eBPF are Linux-only)")
}

func TraceExecSourceWithIdentity(ctx context.Context, opts Options, onAttached func(error), emit func(Event, RuntimeIdentity)) error {
	return fmt.Errorf("tracer.TraceExecSourceWithIdentity: not supported on this platform (Landlock/eBPF are Linux-only)")
}

func TraceConnectSourceWithIdentity(ctx context.Context, opts Options, onAttached func(error), emit func(Event, RuntimeIdentity)) error {
	return fmt.Errorf("tracer.TraceConnectSourceWithIdentity: not supported on this platform (Landlock/eBPF are Linux-only)")
}

func TraceBindSourceWithIdentity(ctx context.Context, opts Options, onAttached func(error), emit func(Event, RuntimeIdentity)) error {
	return fmt.Errorf("tracer.TraceBindSourceWithIdentity: not supported on this platform (Landlock/eBPF are Linux-only)")
}

func TraceCapabilitiesSourceWithIdentity(ctx context.Context, opts Options, onAttached func(error), emit func(Event, RuntimeIdentity)) error {
	return fmt.Errorf("tracer.TraceCapabilitiesSourceWithIdentity: not supported on this platform (Landlock/eBPF are Linux-only)")
}
