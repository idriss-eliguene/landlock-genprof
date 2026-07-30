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
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
