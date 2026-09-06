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
	return unsupportedSource(onAttached, "TraceFilesystemSourceWithIdentity")
}

func TraceExecSourceWithIdentity(ctx context.Context, opts Options, onAttached func(error), emit func(Event, RuntimeIdentity)) error {
	return unsupportedSource(onAttached, "TraceExecSourceWithIdentity")
}

func TraceConnectSourceWithIdentity(ctx context.Context, opts Options, onAttached func(error), emit func(Event, RuntimeIdentity)) error {
	return unsupportedSource(onAttached, "TraceConnectSourceWithIdentity")
}

func TraceBindSourceWithIdentity(ctx context.Context, opts Options, onAttached func(error), emit func(Event, RuntimeIdentity)) error {
	return unsupportedSource(onAttached, "TraceBindSourceWithIdentity")
}

func TraceCapabilitiesSourceWithIdentity(ctx context.Context, opts Options, onAttached func(error), emit func(Event, RuntimeIdentity)) error {
	return unsupportedSource(onAttached, "TraceCapabilitiesSourceWithIdentity")
}

func unsupportedSource(onAttached func(error), source string) error {
	err := fmt.Errorf("tracer.%s: not supported on this platform (Landlock/eBPF are Linux-only)", source)
	if onAttached != nil {
		onAttached(err)
	}
	return err
}
