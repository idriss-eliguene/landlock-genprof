// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package profile

import (
	"go/build"
	"strings"
	"testing"
)

// TestNoOutputFormatDependency guards the IR's core invariant: it must
// stay independent of any specific output/enforcement technology
// (PodLock, YAML, Kubernetes, Cilium, the eBPF collectors, ...). The
// dependency direction is exporter -> profile, never the other way (see
// docs/architecture.md) — nothing at the Go type level stops an
// accidental import in the wrong direction, so it's enforced here
// instead, statically, without needing to build or run anything.
func TestNoOutputFormatDependency(t *testing.T) {
	// build.ImportDir only reports the imports of the package's regular
	// (non-_test.go) files — exactly what we want to check, since this
	// test file's own imports (go/build, strings, testing) shouldn't
	// trip the check.
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("build.ImportDir(%q) error = %v", ".", err)
	}

	forbidden := []string{
		"podlock", // pkg/podlock and internal/exporter/podlock
		"sigs.k8s.io/yaml",
		"k8s.io/",
		"github.com/cilium/",
		"github.com/inspektor-gadget/",
	}

	for _, imp := range pkg.Imports {
		for _, bad := range forbidden {
			if strings.Contains(imp, bad) {
				t.Errorf("internal/profile imports %q (matches forbidden %q) — "+
					"the IR must stay independent of output formats, Kubernetes, and collectors",
					imp, bad)
			}
		}
	}
}
