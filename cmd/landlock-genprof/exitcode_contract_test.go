// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// commandsWithFindingContract classifies every leaf command (one with its
// own RunE — not a bare namespace like `abi`/`evidence`/`policy`
// themselves) against ADR-0001's exit-code contract:
//
//   - true:  this command distinguishes a usage error (exit 3) from a
//     blocking/non-blocking finding (2/1) — its own *_test.go file is
//     expected to assert ExitCode() for at least one usage-error path.
//   - false: this command has no finding/usage-error distinction to make
//     — either it has no error path at all (abi list, version), or every
//     error path is "the operation failed," not "a check ran and found
//     something" (trace, review, apply-proposal, approve, reject, policy
//     list). A plain error (exit 1 by main()'s own default) is correct
//     here, not a gap.
//
// This is not a claim that every "true" command's coverage is complete —
// see each command's own *_test.go for what's actually asserted. It's a
// tripwire: TestCommandTree_EveryLeafCommandIsClassified fails the
// moment a new leaf command exists that isn't in this map at all, which
// is exactly the class of gap that let `verify`'s own usage errors ship
// as exit 1 instead of 3 for a while (see ADR-0001).
var commandsWithFindingContract = map[string]bool{
	"trace":                  false,
	"synthesize":             true,
	"review":                 false,
	"apply-proposal":         false,
	"rollback":               false,
	"custody-epoch activate": false,
	"approve":                false,
	"reject":                 false,
	"doctor":                 true,
	"abi list":               false,
	"abi check":              true,
	"verify":                 true,
	"explain":                true,
	"export":                 true,
	"diff":                   true,
	"evidence show":          true,
	"evidence list":          true,
	"policy list":            false,
	"policy status":          true,
	"ui":                     false,
	"version":                false,
}

// cobraBuiltins are subcommands cobra itself registers (not this
// project's own commands) — excluded from classification, same reason
// they were never in scope for ADR-0001 to begin with.
var cobraBuiltins = map[string]bool{
	"completion": true,
	"help":       true,
}

// TestCommandTree_EveryLeafCommandIsClassified walks the real command
// tree via cobra's own public Commands() API — no reflection, nothing
// beyond what `root.go` already builds — and fails if a leaf command
// exists that commandsWithFindingContract doesn't know about. Adding a
// new command without updating this map is exactly the failure mode
// this test exists to catch: it forces an explicit true/false decision
// at the moment a command is added, rather than letting the question go
// unasked until a CI consumer discovers the wrong exit code in
// production.
func TestCommandTree_EveryLeafCommandIsClassified(t *testing.T) {
	var leaves []string
	var walk func(names []string, cmds []*cobra.Command)
	walk = func(names []string, cmds []*cobra.Command) {
		for _, c := range cmds {
			path := append(append([]string{}, names...), c.Name())
			if len(c.Commands()) == 0 {
				leaves = append(leaves, strings.Join(path, " "))
				continue
			}
			walk(path, c.Commands())
		}
	}
	walk(nil, newRootCmd().Commands())

	for _, name := range leaves {
		if cobraBuiltins[name] {
			continue
		}
		if _, known := commandsWithFindingContract[name]; !known {
			t.Errorf("command %q is not classified in commandsWithFindingContract — "+
				"add it as true (needs exit-code-3 usage-error test coverage, see "+
				"ADR-0001) or false (plain action command, the contract doesn't apply) "+
				"before merging", name)
		}
	}

	// Catches the opposite drift too: an entry in the map for a command
	// that no longer exists (renamed or removed) would otherwise sit
	// there silently, giving a false sense of coverage.
	found := make(map[string]bool, len(leaves))
	for _, name := range leaves {
		found[name] = true
	}
	for name := range commandsWithFindingContract {
		if !found[name] {
			t.Errorf("commandsWithFindingContract lists %q, but no such command exists "+
				"in the current tree — remove the stale entry", name)
		}
	}
}
