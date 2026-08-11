package evidence

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

func mustTime(y int, m time.Month, d, hh, mm, ss int) time.Time {
	return time.Date(y, m, d, hh, mm, ss, 123456789, time.UTC) // include ns to test fidelity
}

func TestV1RoundTripViaDecodeAndFromJSON(t *testing.T) {
	events := []tracer.Event{
		{Timestamp: mustTime(2026, time.August, 11, 12, 0, 0), Syscall: "openat", Path: "/a", Mode: "read"},
		{Timestamp: mustTime(2026, time.August, 11, 12, 0, 1), Syscall: "openat", Path: "/b", Mode: "write"},
	}
	arch := []string{"SCMP_ARCH_X86_64"}
	data, err := ToJSON(events, arch)
	if err != nil {
		t.Fatalf("ToJSON error: %v", err)
	}

	// Decode
	doc, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode v1 error: %v", err)
	}
	if doc.Version != schemaVersionV1 {
		t.Fatalf("expected v1 version, got %q", doc.Version)
	}
	if doc.Run != nil {
		t.Fatalf("expected nil Run for v1, got %+v", doc.Run)
	}
	if len(doc.Events) != len(events) {
		t.Fatalf("expected %d events, got %d", len(events), len(doc.Events))
	}

	// FromJSON compatibility
	e2, a2, err := FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON error: %v", err)
	}
	if len(e2) != len(events) {
		t.Fatalf("FromJSON events len mismatch: %d vs %d", len(e2), len(events))
	}
	if len(a2) != len(arch) {
		t.Fatalf("architectures mismatch: %v vs %v", a2, arch)
	}
}

func TestV2RoundTripDecodeEncode(t *testing.T) {
	events := []tracer.Event{{Timestamp: mustTime(2026, time.August, 11, 12, 1, 0), Syscall: "connect", Port: 443, Mode: "egress"}}
	arch := []string{"SCMP_ARCH_X86_64"}
	run := RunMetadata{
		Source:     "landlock-genprof",
		RecordTime: time.Now().UTC(),
	}
	data, err := ToJSONWithRun(events, arch, run)
	if err != nil {
		t.Fatalf("ToJSONWithRun error: %v", err)
	}

	doc, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode v2 error: %v", err)
	}
	if doc.Version != schemaVersionV2 {
		t.Fatalf("expected v2, got %q", doc.Version)
	}
	if doc.Run == nil {
		t.Fatalf("expected Run metadata for v2")
	}
	if doc.Run.Source != run.Source {
		t.Fatalf("source mismatch: %q vs %q", doc.Run.Source, run.Source)
	}
	if !doc.Run.RecordTime.Equal(run.RecordTime) {
		t.Fatalf("recordTime mismatch: %v vs %v", doc.Run.RecordTime, run.RecordTime)
	}
	if len(doc.Events) != 1 || doc.Events[0].Port != 443 {
		t.Fatalf("event mismatch: %+v", doc.Events)
	}
}

func TestProvenanceRoundTripDecodeEncode(t *testing.T) {
	// build a v2 document with provenanceSources and events referencing them
	d := Document{
		Version: schemaVersionV2,
		Run: &RunMetadata{
			Source:     "landlock-genprof",
			RecordTime: time.Now().UTC(),
		},
		Architectures: []string{"SCMP_ARCH_X86_64"},
		ProvenanceSources: map[string]ProvenanceSource{
			"p1": {BackendKind: "filesystem", OriginType: OriginDirect, BackendAgentID: "agent-a"},
			"p2": {BackendKind: "network", OriginType: OriginDirect},
		},
		Events: []Event{{Timestamp: mustTime(2026, time.August, 11, 12, 0, 0), Syscall: "openat", Path: "/a", Mode: "read", ProvenanceID: "p1"},
			{Timestamp: mustTime(2026, time.August, 11, 12, 0, 1), Syscall: "connect", Port: 80, Mode: "egress", ProvenanceID: "p2"}},
	}
	b, err := Encode(d)
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}
	doc, err := Decode(b)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if len(doc.ProvenanceSources) != 2 {
		t.Fatalf("expected 2 provenance sources, got %d", len(doc.ProvenanceSources))
	}
	if doc.Events[0].ProvenanceID != "p1" || doc.Events[1].ProvenanceID != "p2" {
		t.Fatalf("provenance id mismatch: %+v", doc.Events)
	}
}

func TestProvenanceDanglingReject(t *testing.T) {
	// build a v2 document with an event referencing missing provenance
	raw := `{"version":"v2","run":{"source":"landlock-genprof","recordTime":"2026-08-11T12:00:00Z"},"events":[{"syscall":"openat","path":"/a","provenanceId":"missing"}]}`
	_, err := Decode([]byte(raw))
	if err == nil {
		t.Fatalf("expected Decode to reject dangling provenance reference")
	}
}

func TestProvenanceNonInterference(t *testing.T) {
	// two documents identical in events but differing only by provenance
	events := []tracer.Event{{Timestamp: mustTime(2026, time.August, 11, 12, 0, 0), Syscall: "openat", Path: "/a", Mode: "read"}}
	arch := []string{"SCMP_ARCH_X86_64"}
	run := RunMetadata{Source: "landlock-genprof", RecordTime: time.Now().UTC()}
	b1, err := ToJSONWithRun(events, arch, run)
	if err != nil {
		t.Fatalf("ToJSONWithRun error: %v", err)
	}
	// doc2: same events but with provenance wrapped
	d2 := Document{}
	d2.Version = schemaVersionV2
	d2.Run = &run
	d2.Architectures = arch
	d2.ProvenanceSources = map[string]ProvenanceSource{"p1": {BackendKind: "filesystem", OriginType: OriginDirect, BackendAgentID: "agent-a"}}
	d2.Events = []Event{{Timestamp: mustTime(2026, time.August, 11, 12, 0, 0), Syscall: "openat", Path: "/a", Mode: "read", ProvenanceID: "p1"}}
	b2, err := Encode(d2)
	if err != nil {
		t.Fatalf("Encode d2 error: %v", err)
	}
	// FromJSON should return identical tracer.Events for both
	e1, _, err := FromJSON(b1)
	if err != nil {
		t.Fatalf("FromJSON b1 error: %v", err)
	}
	e2, _, err := FromJSON(b2)
	if err != nil {
		t.Fatalf("FromJSON b2 error: %v", err)
	}
	if len(e1) != len(e2) || e1[0].Syscall != e2[0].Syscall || e1[0].Path != e2[0].Path {
		t.Fatalf("events differ after adding provenance: %v vs %v", e1, e2)
	}
}

func TestStartEndValidation(t *testing.T) {
	events := []tracer.Event{}
	arch := []string{}
	start := mustTime(2026, time.August, 11, 12, 5, 0)
	end := mustTime(2026, time.August, 11, 12, 4, 0)
	run := RunMetadata{Source: "s", RecordTime: time.Now().UTC(), Start: &start, End: &end}
	_, err := ToJSONWithRun(events, arch, run)
	if err == nil {
		// Encode will succeed, but Decode should reject when reading
		// an invalid Start > End document.
		data, _ := ToJSONWithRun(events, arch, run)
		_, derr := Decode(data)
		if derr == nil {
			t.Fatalf("expected Decode to reject Start > End")
		}
	}
}

func TestUnsupportedVersionAndCorruptJSON(t *testing.T) {
	// unsupported version
	raw := []byte(`{"version":"v3","events":[]}`)
	_, err := Decode(raw)
	if err == nil {
		t.Fatalf("expected error for unsupported version v3")
	}

	// corrupt JSON
	_, err = Decode([]byte("not json"))
	if err == nil {
		t.Fatalf("expected error for corrupt JSON")
	}
}

func TestEventOrderAndFieldPreservation(t *testing.T) {
	events := []tracer.Event{
		{Timestamp: mustTime(2026, time.August, 11, 12, 0, 0), Syscall: "openat", Path: "/a", Mode: "read"},
		{Timestamp: mustTime(2026, time.August, 11, 12, 0, 1), Syscall: "connect", Port: 80, Mode: "egress"},
		{Timestamp: mustTime(2026, time.August, 11, 12, 0, 0), Syscall: "openat", Path: "/a", Mode: "read"},
	}
	arch := []string{"SCMP_ARCH_X86_64"}
	data, err := ToJSON(events, arch)
	if err != nil {
		t.Fatalf("ToJSON error: %v", err)
	}
	doc, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if len(doc.Events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(doc.Events))
	}
	// check order and sample fields
	if doc.Events[0].Path != "/a" || doc.Events[1].Port != 80 || doc.Events[2].Path != "/a" {
		t.Fatalf("event fields/order not preserved: %+v", doc.Events)
	}
}

func TestRecordTimeNanosecondsPrecision(t *testing.T) {
	rt := time.Date(2026, time.August, 11, 12, 0, 0, 987654321, time.UTC)
	run := RunMetadata{Source: "s", RecordTime: rt}
	data, err := ToJSONWithRun(nil, nil, run)
	if err != nil {
		t.Fatalf("ToJSONWithRun error: %v", err)
	}
	doc, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if !doc.Run.RecordTime.Equal(rt) {
		t.Fatalf("recordTimeprecision mismatch: %v vs %v", doc.Run.RecordTime, rt)
	}
}

func TestFromJSONCompatibilityWithV1(t *testing.T) {
	events := []tracer.Event{{Syscall: "openat", Path: "/etc/passwd", Mode: "read"}}
	data, err := ToJSON(events, nil)
	if err != nil {
		t.Fatalf("ToJSON error: %v", err)
	}
	// FromJSON should still work
	res, _, err := FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON error: %v", err)
	}
	if len(res) != 1 || res[0].Path != "/etc/passwd" {
		t.Fatalf("FromJSON roundtrip failed: %+v", res)
	}
}

func TestEncodeDeterministic(t *testing.T) {
	e := tracer.Event{Syscall: "openat", Path: "/a", Mode: "read", Timestamp: mustTime(2026, time.August, 11, 12, 0, 0)}
	run := RunMetadata{Source: "s", RecordTime: time.Now().UTC()}
	b1, err := ToJSONWithRun([]tracer.Event{e}, nil, run)
	if err != nil {
		t.Fatalf("ToJSONWithRun error: %v", err)
	}
	b2, err := ToJSONWithRun([]tracer.Event{e}, nil, run)
	if err != nil {
		t.Fatalf("ToJSONWithRun error: %v", err)
	}
	if string(b1) != string(b2) {
		// MarshalIndent deterministic for same input; fail if not
		t.Fatalf("expected deterministic encoding, got different bytes")
	}
	// Also ensure JSON unmarshals back to same Document
	var d1, d2 Document
	if err := json.Unmarshal(b1, &d1); err != nil {
		t.Fatalf("unmarshal d1: %v", err)
	}
	if err := json.Unmarshal(b2, &d2); err != nil {
		t.Fatalf("unmarshal d2: %v", err)
	}
	if d1.Version != d2.Version || d1.Run.Source != d2.Run.Source {
		t.Fatalf("documents differ after marshal/unmarshal")
	}
}
