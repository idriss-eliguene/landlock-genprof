// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

//go:build !gendocs

// Command landlock-genprof observes a running Kubernetes pod and generates
// least-privilege security profiles from what it actually saw: a
// PodLock LandlockProfile always, plus NetworkPolicy/seccomp/Linux
// capabilities/securityContext outputs behind their own flags.
//
// Usage:
//
//	landlock-genprof trace --pod <name> --namespace <ns> --binary <path> --duration 60s --out profile.yaml
package main

import (
	"fmt"
	"os"
)

func main() {
	err := newRootCmd().Execute()
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)

	// A command may return an error that also carries a specific exit
	// code (see doctor.go's doctorExitError) — the exit-code contract
	// docs/cli-design.md commits to for verify/diff starts here, on the
	// cheapest command, so main() already supports it once something
	// CI-critical needs it. A plain error (the common case today) still
	// exits 1, unchanged.
	if exitCoder, ok := err.(interface{ ExitCode() int }); ok {
		os.Exit(exitCoder.ExitCode())
	}
	os.Exit(1)
}
