// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package landlock

// ABILevel is a Landlock ABI version — the kernel's own compatibility
// unit for what a Landlock ruleset can express. The authoritative way to
// detect it is at runtime, via
// landlock_create_ruleset(NULL, 0, LANDLOCK_CREATE_RULESET_VERSION) —
// confirmed against docs.kernel.org's own landlock.rst, which
// explicitly recommends this over any kernel-version heuristic (a
// distro can backport support). MinKernelFor below is a documented
// approximation for the cases a live syscall isn't possible (checking a
// fleet's node-pool version before it's provisioned) — not a substitute
// for the real check where one is available.
type ABILevel int

const (
	ABI1 ABILevel = 1
	ABI2 ABILevel = 2
	ABI3 ABILevel = 3
	ABI4 ABILevel = 4
	ABI5 ABILevel = 5
	ABI6 ABILevel = 6
	ABI7 ABILevel = 7
)

// LandlockRight is one real Landlock access/scope right, named per the
// kernel's own LANDLOCK_ACCESS_*/LANDLOCK_SCOPE_* vocabulary.
//
// Rule.Rights (kernel.go) carries LandlockRight values directly — but
// only 4 of the ~19 rights this file knows about are ever actually
// produced by Synthesize today (LandlockRightReadFile/ReadDir/WriteFile/
// Execute, all ABI1), since that's the ceiling of what a
// FilesystemObservation's Operation+IsDir bits can honestly distinguish.
// This file's own data — the full ABI table — also backs the `abi`
// command standalone, independent of any specific synthesized candidate;
// see docs/landlock-kernel-extraction.md's "known gap" section for
// exactly what's produced today versus what would need new tracer
// capture (unlink/mkdir/rename/... syscalls this project doesn't observe
// yet) to go further.
type LandlockRight string

// Named with an explicit LandlockRight prefix for clarity at every call
// site, given how many of these constants this package and its consumers
// end up naming side by side.
const (
	LandlockRightExecute    LandlockRight = "execute"
	LandlockRightWriteFile  LandlockRight = "write_file"
	LandlockRightReadFile   LandlockRight = "read_file"
	LandlockRightReadDir    LandlockRight = "read_dir"
	LandlockRightRemoveDir  LandlockRight = "remove_dir"
	LandlockRightRemoveFile LandlockRight = "remove_file"
	LandlockRightMakeChar   LandlockRight = "make_char"
	LandlockRightMakeDir    LandlockRight = "make_dir"
	LandlockRightMakeReg    LandlockRight = "make_reg"
	LandlockRightMakeSock   LandlockRight = "make_sock"
	LandlockRightMakeFifo   LandlockRight = "make_fifo"
	LandlockRightMakeBlock  LandlockRight = "make_block"
	LandlockRightMakeSym    LandlockRight = "make_sym"

	LandlockRightRefer    LandlockRight = "refer"    // ABI 2 — link/rename across directories
	LandlockRightTruncate LandlockRight = "truncate" // ABI 3

	LandlockRightNetBindTCP    LandlockRight = "net_bind_tcp"    // ABI 4
	LandlockRightNetConnectTCP LandlockRight = "net_connect_tcp" // ABI 4

	LandlockRightIoctlDev LandlockRight = "ioctl_dev" // ABI 5

	LandlockRightScopeAbstractUnixSocket LandlockRight = "scope_abstract_unix_socket" // ABI 6
	LandlockRightScopeSignal             LandlockRight = "scope_signal"               // ABI 6
)

// abiOf maps each LandlockRight to the ABI level that introduced it.
//
// Confirmed 2026-08-06 against docs.kernel.org/userspace-api/landlock.html
// and man7.org/linux/man-pages/man7/landlock.7.html (ABI 1-6 cross-checked
// against both sources; ABI 7's existence — logging-control flags,
// kernel 6.15 — is confirmed but its own right/flag names aren't part of
// this table yet: they're restrict_self flags, not LANDLOCK_ACCESS_FS_*/
// LANDLOCK_ACCESS_NET_* rights, a different shape this type doesn't
// model. ABI 8/9/10 (thread-sync, AF_UNIX pathname resolution, UDP) are
// confirmed to exist in current kernel documentation but their exact
// kernel-version introduction wasn't confirmed against a primary source
// as of this table's own last-verified date — deliberately left out
// rather than guessed. Re-verify before extending this table forward;
// don't just increment version numbers by pattern-matching the sequence
// above.
var abiOf = map[LandlockRight]ABILevel{
	LandlockRightExecute:    ABI1,
	LandlockRightWriteFile:  ABI1,
	LandlockRightReadFile:   ABI1,
	LandlockRightReadDir:    ABI1,
	LandlockRightRemoveDir:  ABI1,
	LandlockRightRemoveFile: ABI1,
	LandlockRightMakeChar:   ABI1,
	LandlockRightMakeDir:    ABI1,
	LandlockRightMakeReg:    ABI1,
	LandlockRightMakeSock:   ABI1,
	LandlockRightMakeFifo:   ABI1,
	LandlockRightMakeBlock:  ABI1,
	LandlockRightMakeSym:    ABI1,

	LandlockRightRefer:    ABI2,
	LandlockRightTruncate: ABI3,

	LandlockRightNetBindTCP:    ABI4,
	LandlockRightNetConnectTCP: ABI4,

	LandlockRightIoctlDev: ABI5,

	LandlockRightScopeAbstractUnixSocket: ABI6,
	LandlockRightScopeSignal:             ABI6,
}

// kernelVersion is a bare major.minor pair — deliberately not reusing
// cmd/landlock-genprof's own kernel-parsing types: this package stays
// free of any cmd/ dependency (the reverse direction is the only one
// that should ever exist), so it defines the tiny shape it needs itself.
type kernelVersion struct{ Major, Minor int }

// minKernelForABI is the documented kernel-version approximation this
// file's own package doc warns about — same "confirmed against
// docs.kernel.org/man7.org, 2026-08-06" provenance as abiOf.
var minKernelForABI = map[ABILevel]kernelVersion{
	ABI1: {5, 13},
	ABI2: {5, 19},
	ABI3: {6, 2},
	ABI4: {6, 7},
	ABI5: {6, 10},
	ABI6: {6, 12},
	ABI7: {6, 15},
}

// RightsAt returns every LandlockRight available at or below abi,
// sorted by the ABI level that introduced them (ties broken by name for
// deterministic output).
func RightsAt(abi ABILevel) []LandlockRight {
	var rights []LandlockRight
	for right, introducedAt := range abiOf {
		if introducedAt <= abi {
			rights = append(rights, right)
		}
	}
	sortRightsDeterministic(rights)
	return rights
}

// ABIForRight returns the ABI level that introduced right, and whether
// right is known to this table at all.
func ABIForRight(right LandlockRight) (ABILevel, bool) {
	abi, ok := abiOf[right]
	return abi, ok
}

// MinKernelFor returns the documented minimum kernel version
// (major, minor) for abi, and whether abi is known to this table — see
// the package doc for why this is an approximation, not the
// authoritative detection method.
func MinKernelFor(abi ABILevel) (major, minor int, ok bool) {
	v, ok := minKernelForABI[abi]
	return v.Major, v.Minor, ok
}

// ABIForKernel returns the highest ABILevel whose documented minimum
// kernel version is satisfied by major.minor, and false if the kernel
// predates even ABI1 (Landlock unsupported at all).
func ABIForKernel(major, minor int) (ABILevel, bool) {
	var best ABILevel
	found := false
	for abi, min := range minKernelForABI {
		if kernelAtLeastVersion(major, minor, min.Major, min.Minor) {
			if !found || abi > best {
				best = abi
				found = true
			}
		}
	}
	return best, found
}

// HighestABI returns the highest ABILevel required among rights, and
// false if rights is empty or contains no right known to this table —
// the "what does this candidate actually need" counterpart to RightsAt's
// "what does this kernel actually support," used by `verify` to report
// which specific ABI level an incompatible rule is blocked on.
func HighestABI(rights []LandlockRight) (ABILevel, bool) {
	var highest ABILevel
	found := false
	for _, right := range rights {
		abi, ok := abiOf[right]
		if !ok {
			continue
		}
		if !found || abi > highest {
			highest = abi
			found = true
		}
	}
	return highest, found
}

func kernelAtLeastVersion(major, minor, minMajor, minMinor int) bool {
	if major != minMajor {
		return major > minMajor
	}
	return minor >= minMinor
}

func sortRightsDeterministic(rights []LandlockRight) {
	// Insertion sort is plenty for a list this small (currently 19
	// rights total) and avoids importing "sort" for one call site.
	for i := 1; i < len(rights); i++ {
		for j := i; j > 0 && lessRight(rights[j], rights[j-1]); j-- {
			rights[j], rights[j-1] = rights[j-1], rights[j]
		}
	}
}

func lessRight(a, b LandlockRight) bool {
	if abiOf[a] != abiOf[b] {
		return abiOf[a] < abiOf[b]
	}
	return a < b
}
