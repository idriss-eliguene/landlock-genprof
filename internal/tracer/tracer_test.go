package tracer

import (
	"testing"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation"
)

func TestIsFilesystemEvent_Cases(t *testing.T) {
	tests := []struct {
		name string
		ev   Event
		want bool
	}{
		{"absolute read", Event{Path: "/etc/hosts", Mode: "read"}, true},
		{"absolute write", Event{Path: "/tmp/x", Mode: "write"}, true},
		{"absolute read_write", Event{Path: "/tmp/x", Mode: "read_write"}, true},
		{"absolute exec", Event{Path: "/bin/sh", Mode: "exec"}, true},
		{"empty path", Event{Path: "", Mode: "read"}, false},
		{"relative path", Event{Path: "relative/p", Mode: "read"}, false},
		{"egress mode", Event{Path: "/some", Mode: "egress"}, false},
		{"ingress mode", Event{Path: "/some", Mode: "ingress"}, false},
		{"capability-like", Event{Path: "/some", Mode: "capability"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsFilesystemEvent(tt.ev)
			if got != tt.want {
				t.Fatalf("IsFilesystemEvent(%v) = %v, want %v", tt.ev, got, tt.want)
			}
		})
	}
}

// TestToObservation_SeccompAdvisoryEvents_ClassifiedAsSyscall guards
// against a regression found via diagnostic run 32028412953: advise_seccomp
// advisory events (Mode == "syscall") whose Syscall field happens to name a
// network syscall (bind, connect, ...) must be classified KindSyscall, not
// KindNetwork. Before the fix, the network Syscall-name check ran first and
// permanently claimed the event, so the Mode == "syscall" check downstream
// never had a chance to run.
func TestToObservation_SeccompAdvisoryEvents_ClassifiedAsSyscall(t *testing.T) {
	cases := []struct {
		name    string
		syscall string
	}{
		{"bind advisory event", "bind"},
		{"connect advisory event", "connect"},
		{"sendmsg advisory event", "sendmsg"},
		{"recvmsg advisory event", "recvmsg"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ev := Event{
				Syscall: c.syscall,
				Mode:    "syscall",
			}
			obs := ToObservation(ev)
			if obs.Kind != observation.KindSyscall {
				t.Errorf("ToObservation(%+v).Kind = %v, want KindSyscall", ev, obs.Kind)
			}
		})
	}
}

// TestToObservation_RealNetworkEvents_StillClassifiedAsNetwork is the
// companion regression guard: the reorder must not break legitimate
// network classification for events that are NOT seccomp advisory events
// (i.e. Mode is not "syscall").
func TestToObservation_RealNetworkEvents_StillClassifiedAsNetwork(t *testing.T) {
	cases := []struct {
		name string
		ev   Event
	}{
		{"bind via network mode", Event{Syscall: "bind", Mode: "egress"}},
		{"connect with empty mode", Event{Syscall: "connect", Mode: ""}},
		{"ingress mode", Event{Syscall: "", Mode: "ingress"}},
		{"egress mode", Event{Syscall: "", Mode: "egress"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			obs := ToObservation(c.ev)
			if obs.Kind != observation.KindNetwork {
				t.Errorf("ToObservation(%+v).Kind = %v, want KindNetwork", c.ev, obs.Kind)
			}
		})
	}
}

// TestToObservation_CapabilityMode_Unaffected guards the sibling
// Mode == "capability" branch, which moved alongside Mode == "syscall" in
// the same reorder and must keep behaving identically.
func TestToObservation_CapabilityMode_Unaffected(t *testing.T) {
	ev := Event{Syscall: "bind", Mode: "capability"}
	obs := ToObservation(ev)
	if obs.Kind != observation.KindCapability {
		t.Errorf("ToObservation(%+v).Kind = %v, want KindCapability", ev, obs.Kind)
	}
}

// TestToObservation_Filesystem_StillTakesPriority guards that filesystem
// classification, which runs before the reordered block, is unaffected.
func TestToObservation_Filesystem_StillTakesPriority(t *testing.T) {
	ev := Event{Path: "/etc/passwd", Mode: "read", Syscall: "openat"}
	obs := ToObservation(ev)
	if obs.Kind != observation.KindFilesystem {
		t.Errorf("ToObservation(%+v).Kind = %v, want KindFilesystem", ev, obs.Kind)
	}
}
