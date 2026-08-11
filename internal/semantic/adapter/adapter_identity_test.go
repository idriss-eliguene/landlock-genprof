package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/observation"
	"github.com/idriss-eliguene/landlock-genprof/internal/semantic"
	"github.com/idriss-eliguene/landlock-genprof/internal/tracer"
)

// Test-only snapshot DTOs (do not export from production)
type projectionSnapshot struct {
	ActIdentity    string              `json:"act_identity"`
	Assertions     []assertionSnapshot `json:"assertions"`
	EvidenceGroups map[string][]int    `json:"evidence_groups"`
}

type assertionSnapshot struct {
	AssertionID          string `json:"assertion_id"`
	PropositionCanonical string `json:"proposition_canonical"`
	RecordTime           string `json:"record_time"`
	BeliefStatus         int    `json:"belief_status"`
}

func loadGolden(t *testing.T, name string) projectionSnapshot {
	t.Helper()
	p := filepath.Join("testdata", "golden", name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read golden %s: %v", p, err)
	}
	var snap projectionSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		t.Fatalf("unmarshal golden %s: %v", p, err)
	}
	return snap
}

func compareSnapshots(t *testing.T, expected projectionSnapshot, actual projectionSnapshot) {
	t.Helper()
	if expected.ActIdentity != actual.ActIdentity {
		t.Fatalf("ActIdentity mismatch: expected %q got %q", expected.ActIdentity, actual.ActIdentity)
	}
	if len(expected.Assertions) != len(actual.Assertions) {
		t.Fatalf("assertion count mismatch: expected %d got %d", len(expected.Assertions), len(actual.Assertions))
	}
	// compare assertions by position
	for i := range expected.Assertions {
		e := expected.Assertions[i]
		a := actual.Assertions[i]
		if e.AssertionID != a.AssertionID {
			t.Fatalf("assertion id mismatch at %d: expected %q got %q", i, e.AssertionID, a.AssertionID)
		}
		if e.PropositionCanonical != a.PropositionCanonical {
			t.Fatalf("prop canonical mismatch for %s: expected %q got %q", e.AssertionID, e.PropositionCanonical, a.PropositionCanonical)
		}
		if e.RecordTime != a.RecordTime {
			t.Fatalf("record time mismatch for %s: expected %q got %q", e.AssertionID, e.RecordTime, a.RecordTime)
		}
		if e.BeliefStatus != a.BeliefStatus {
			t.Fatalf("belief status mismatch for %s: expected %d got %d", e.AssertionID, e.BeliefStatus, a.BeliefStatus)
		}
	}
	// compare EvidenceGroups mapping keys and values
	if len(expected.EvidenceGroups) != len(actual.EvidenceGroups) {
		t.Fatalf("evidence groups size mismatch: expected %d got %d", len(expected.EvidenceGroups), len(actual.EvidenceGroups))
	}
	for k, ev := range expected.EvidenceGroups {
		actEv, ok := actual.EvidenceGroups[k]
		if !ok {
			t.Fatalf("expected evidence group key %s missing in actual", k)
		}
		if len(ev) != len(actEv) {
			t.Fatalf("evidence group length mismatch for %s: expected %d got %d", k, len(ev), len(actEv))
		}
		for i := range ev {
			if ev[i] != actEv[i] {
				t.Fatalf("evidence index mismatch for %s at pos %d: expected %d got %d", k, i, ev[i], actEv[i])
			}
		}
	}
}

func snapshotFromResult(t *testing.T, res BuildResult) projectionSnapshot {
	t.Helper()
	var snap projectionSnapshot
	snap.EvidenceGroups = make(map[string][]int)
	acts := res.Graph.GetActs()
	if len(acts) > 0 {
		snap.ActIdentity = acts[0].Identity().IdentityString()
	}
	L, err := res.Graph.BeliefState()
	if err != nil {
		t.Fatalf("BeliefState error: %v", err)
	}
	for _, id := range res.AssertionIDs {
		ev, ok := res.Graph.GetAssertionEvent(id)
		if !ok {
			t.Fatalf("assertion %s not found in graph", id)
		}

		rt := ev.RecordTime()
		prop := ev.Proposition()
		a := assertionSnapshot{
			AssertionID:          string(id),
			PropositionCanonical: semantic.CanonicalString(prop),
			RecordTime:           rt.Time().UTC().Format("2006-01-02T15:04:05Z"),
			BeliefStatus:         int(L.Lookup(id)),
		}
		snap.Assertions = append(snap.Assertions, a)
		snap.EvidenceGroups[string(id)] = append([]int(nil), res.EvidenceGroups[id]...)
	}
	return snap
}

func Test_IdentityFreeze_Filesystem(t *testing.T) {
	exp := loadGolden(t, "filesystem.json")
	meta := RunMeta{Source: semantic.NewSubjectIdentity("src-fs"), RecordTime: mustTime(t, 2026, time.January, 1, 0, 0, 10)}
	e1 := observation.Observation{Kind: observation.KindFilesystem, Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 1), Syscall: "read", Path: "/x", Mode: "read"}
	e2 := observation.Observation{Kind: observation.KindFilesystem, Timestamp: mustTime(t, 2026, time.January, 1, 0, 0, 2), Syscall: "read", Path: "/x", Mode: "read"}
	res, err := BuildGraphFromObservations(meta, []observation.Observation{e1, e2})
	if err != nil {
		t.Fatalf("BuildGraphFromObservations error: %v", err)
	}
	act := snapshotFromResult(t, res)
	compareSnapshots(t, exp, act)
}

func Test_IdentityFreeze_Network(t *testing.T) {
	exp := loadGolden(t, "network.json")
	meta := RunMeta{Source: semantic.NewSubjectIdentity("src-net"), RecordTime: mustTime(t, 2026, time.January, 1, 1, 0, 0)}
	c := observation.Observation{Kind: observation.KindNetwork, Timestamp: mustTime(t, 2026, time.January, 1, 1, 0, 1), Syscall: "connect", Mode: "egress", Port: 443}
	b := observation.Observation{Kind: observation.KindNetwork, Timestamp: mustTime(t, 2026, time.January, 1, 1, 0, 2), Syscall: "bind", Mode: "ingress", Port: 443}
	res, err := BuildGraphFromObservations(meta, []observation.Observation{c, b})
	if err != nil {
		t.Fatalf("BuildGraphFromObservations error: %v", err)
	}
	act := snapshotFromResult(t, res)
	compareSnapshots(t, exp, act)
}

func Test_IdentityFreeze_Capability(t *testing.T) {
	exp := loadGolden(t, "capability.json")
	meta := RunMeta{Source: semantic.NewSubjectIdentity("src-cap"), RecordTime: mustTime(t, 2026, time.January, 1, 2, 0, 0)}
	c1 := observation.Observation{Kind: observation.KindCapability, Timestamp: mustTime(t, 2026, time.January, 1, 2, 0, 1), Syscall: "CAP_NET_BIND_SERVICE", Mode: "capability"}
	c2 := observation.Observation{Kind: observation.KindCapability, Timestamp: mustTime(t, 2026, time.January, 1, 2, 0, 2), Syscall: "CAP_NET_BIND_SERVICE", Mode: "capability"}
	res, err := BuildGraphFromObservations(meta, []observation.Observation{c1, c2})
	if err != nil {
		t.Fatalf("BuildGraphFromObservations error: %v", err)
	}
	act := snapshotFromResult(t, res)
	compareSnapshots(t, exp, act)
}

func Test_IdentityFreeze_Syscall(t *testing.T) {
	exp := loadGolden(t, "syscall.json")
	meta := RunMeta{Source: semantic.NewSubjectIdentity("src-sys"), RecordTime: mustTime(t, 2026, time.January, 1, 3, 0, 0)}
	s1 := observation.Observation{Kind: observation.KindSyscall, Timestamp: time.Time{}, Syscall: "openat", Mode: "syscall"}
	s2 := observation.Observation{Kind: observation.KindSyscall, Timestamp: time.Time{}, Syscall: "openat", Mode: "syscall"}
	res, err := BuildGraphFromObservations(meta, []observation.Observation{s1, s2})
	if err != nil {
		t.Fatalf("BuildGraphFromObservations error: %v", err)
	}
	act := snapshotFromResult(t, res)
	compareSnapshots(t, exp, act)
}

func Test_IdentityFreeze_Mixed(t *testing.T) {
	exp := loadGolden(t, "mixed.json")
	meta := RunMeta{Source: semantic.NewSubjectIdentity("src-mix"), RecordTime: mustTime(t, 2026, time.January, 1, 4, 0, 0)}
	fs := observation.Observation{Kind: observation.KindFilesystem, Timestamp: mustTime(t, 2026, time.January, 1, 4, 0, 1), Syscall: "openat", Path: "/var/log/x", Mode: "read"}
	n := observation.Observation{Kind: observation.KindNetwork, Timestamp: mustTime(t, 2026, time.January, 1, 4, 0, 2), Syscall: "connect", Mode: "egress", Port: 80}
	cap := observation.Observation{Kind: observation.KindCapability, Timestamp: mustTime(t, 2026, time.January, 1, 4, 0, 3), Syscall: "CAP_NET_BIND_SERVICE", Mode: "capability"}
	sc := observation.Observation{Kind: observation.KindSyscall, Timestamp: time.Time{}, Syscall: "execve", Mode: "syscall"}
	res, err := BuildGraphFromObservations(meta, []observation.Observation{fs, n, cap, sc})
	if err != nil {
		t.Fatalf("BuildGraphFromObservations error: %v", err)
	}
	act := snapshotFromResult(t, res)
	compareSnapshots(t, exp, act)
}

func Test_IdentityFreeze_ReplayV2(t *testing.T) {
	exp := loadGolden(t, "replay-v2.json")
	// read evidence v2 doc from testdata/golden/replay-v2.json (same file as golden created)
	// reconstruct same events as used when golden was produced
	evs := []tracer.Event{
		{Timestamp: mustTime(t, 2026, time.January, 1, 5, 0, 1), Syscall: "connect", Mode: "egress", Port: 53},
		{Timestamp: mustTime(t, 2026, time.January, 1, 5, 0, 2), Syscall: "openat", Mode: "read", Path: "/etc/passwd"},
	}
	obs := make([]observation.Observation, 0, len(evs))
	for _, e := range evs {
		obs = append(obs, tracer.ToObservation(e))
	}
	meta := RunMeta{Source: semantic.NewSubjectIdentity("src-replay"), RecordTime: mustTime(t, 2026, time.January, 1, 5, 0, 0)}
	res, err := BuildGraphFromObservations(meta, obs)
	if err != nil {
		t.Fatalf("BuildGraphFromObservations: %v", err)
	}
	act := snapshotFromResult(t, res)
	compareSnapshots(t, exp, act)
}
