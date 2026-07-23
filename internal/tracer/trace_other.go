// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

//go:build !linux

package tracer

import "fmt"

// Trace is not available on non-Linux platforms: Landlock and eBPF, and
// therefore Inspektor Gadget's gadgets, are Linux-only. Build and test
// landlock-genprof from the dev VM (see HOW_TO_START.md) for anything
// touching the tracer — see trace_linux.go for the real implementation.
func Trace(opts Options, onReady func()) ([]Event, error) {
	return nil, fmt.Errorf("tracer.Trace: not supported on this platform (Landlock/eBPF are Linux-only)")
}
