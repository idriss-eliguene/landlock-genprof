// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

// seccomp-verifier-probe is intentionally a single-operation executable. Its
// Pod termination code is the only result channel; no logs or exec API needed.
package main

import (
	"fmt"
	"os"
	"syscall"
)

const (
	// These codes are part of the fixed, versioned Pod termination protocol.
	exitSuccess      = 0
	exitEPERM        = 10
	exitInconclusive = 20
)

func main() { os.Exit(run()) }

func run() int {
	// PRIO_PROCESS and pid 0 refer to the calling process. RawSyscall retains
	// the kernel errno so only an observed EPERM maps to the denial result.
	_, _, errno := syscall.RawSyscall(syscall.SYS_GETPRIORITY, 0, 0, 0)
	switch errno {
	case 0:
		return exitSuccess
	case syscall.EPERM:
		return exitEPERM
	default:
		fmt.Fprintln(os.Stderr, "getpriority probe inconclusive")
		return exitInconclusive
	}
}
