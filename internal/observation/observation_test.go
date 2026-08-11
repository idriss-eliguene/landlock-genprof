package observation

import (
	"testing"
	"time"
)

func TestObservationFieldPreservation(t *testing.T) {
	now := time.Now().UTC()
	obs := Observation{
		Kind:      KindFilesystem,
		Path:      "/etc/passwd",
		Mode:      "read",
		Syscall:   "openat",
		IsDir:     false,
		Truncate:  false,
		Port:      0,
		Timestamp: now,
	}
	// round-trip-like check: fields preserved as assigned
	if obs.Path != "/etc/passwd" || obs.Mode != "read" || obs.Syscall != "openat" {
		t.Fatalf("field preservation failed: %+v", obs)
	}
	if !obs.Timestamp.Equal(now) {
		t.Fatalf("timestamp mismatch: got %v want %v", obs.Timestamp, now)
	}
}

func TestKindConstants(t *testing.T) {
	if KindFilesystem == KindOther || KindNetwork == KindOther {
		t.Fatalf("kind constants not distinct")
	}
}
