package evidence

import (
	"testing"
)

func TestDecode_ExplicitEmptyProvenanceID_Rejected(t *testing.T) {
	json := `{
  "version": "v2",
  "run": { "source": "x", "recordTime": "2026-01-01T00:00:00Z" },
  "events": [ { "syscall": "openat", "provenanceId": "" } ]
}`
	_, err := Decode([]byte(json))
	if err == nil {
		t.Fatalf("expected error for explicit empty provenanceId, got nil")
	}
}

func TestDecode_DuplicateProvenanceSourceKey_Rejected(t *testing.T) {
	json := `{
  "version": "v2",
  "run": { "source": "x", "recordTime": "2026-01-01T00:00:00Z" },
  "provenanceSources": {
    "p1": { "backendKind": "trace_open", "originType": "direct" },
    "p1": { "backendKind": "advise_seccomp", "originType": "advisory" }
  },
  "events": []
}`
	_, err := Decode([]byte(json))
	if err == nil {
		t.Fatalf("expected error for duplicate provenanceSources key, got nil")
	}
}

func TestDecode_DuplicateFieldInProvenanceSource_Rejected(t *testing.T) {
	json := `{
  "version": "v2",
  "run": { "source": "x", "recordTime": "2026-01-01T00:00:00Z" },
  "provenanceSources": {
    "p1": { "backendKind": "trace_open", "backendKind": "evil", "originType": "direct" }
  },
  "events": []
}`
	_, err := Decode([]byte(json))
	if err == nil {
		t.Fatalf("expected error for duplicate field inside provenance source, got nil")
	}
}

func TestDecode_AbsentProvenanceID_Accepted(t *testing.T) {
	json := `{
  "version": "v2",
  "run": { "source": "x", "recordTime": "2026-01-01T00:00:00Z" },
  "events": [ { "syscall": "openat" } ]
}`
	if _, err := Decode([]byte(json)); err != nil {
		t.Fatalf("unexpected error for absent provenanceId: %v", err)
	}
}

func TestDecode_DanglingProvenance_StillRejected(t *testing.T) {
	json := `{
  "version": "v2",
  "run": { "source": "x", "recordTime": "2026-01-01T00:00:00Z" },
  "events": [ { "syscall": "openat", "provenanceId": "p1" } ],
  "provenanceSources": {}
}`
	_, err := Decode([]byte(json))
	if err == nil {
		t.Fatalf("expected error for dangling provenance, got nil")
	}
}

func TestDecode_UnrelatedUnknownField_Preserved(t *testing.T) {
	json := `{
  "version": "v2",
  "run": { "source": "x", "recordTime": "2026-01-01T00:00:00Z" },
  "fooFuture": { "bar": 1 },
  "events": []
}`
	if _, err := Decode([]byte(json)); err != nil {
		t.Fatalf("unexpected error for unknown unrelated field: %v", err)
	}
}
