// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package landlock

import (
	"go/build"
	"strings"
	"testing"
)

// TestNoExternalDependency guards this package's core invariant: it must
// stay independent of Kubernetes, output formats, the cross-domain
// Behavior IR, and the tracer — see the package doc for why each of these
// is excluded on purpose. Mirrors internal/profile/deps_test.go's own
// pattern and reasoning: nothing at the Go type level stops an accidental
// import in the wrong direction, so it's enforced here, statically,
// without needing to build or run anything.
func TestNoExternalDependency(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("build.ImportDir(%q) error = %v", ".", err)
	}

	forbidden := []string{
		"podlock", // pkg/podlock and internal/exporter/podlock
		"landlock-genprof/internal/profile",
		"landlock-genprof/internal/tracer",
		"landlock-genprof/internal/exporter",
		"landlock-genprof/internal/k8s",
		"sigs.k8s.io/yaml",
		"gopkg.in/yaml",
		"k8s.io/",
		"github.com/cilium/",
		"github.com/inspektor-gadget/",
	}

	for _, imp := range pkg.Imports {
		for _, bad := range forbidden {
			if strings.Contains(imp, bad) {
				t.Errorf("internal/landlock imports %q (matches forbidden %q) — "+
					"this package must stay a standalone filesystem-synthesis kernel, "+
					"see the package doc and docs/landlock-kernel-extraction.md",
					imp, bad)
			}
		}
	}
}
