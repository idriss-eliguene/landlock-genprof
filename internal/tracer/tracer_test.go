package tracer

import "testing"

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
