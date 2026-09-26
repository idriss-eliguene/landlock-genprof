package seccompverification

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEvaluateBoundedGetpriority(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	targetID := Identity{ProposalUID: "proposal-uid", CandidateDigest: "sha256:approved", Namespace: "work", ProfileName: "api", ProfileUID: "profile-uid", ProfileDigest: "sha256:profile", WorkloadUID: "deployment-uid", Workload: "Deployment/api", PodName: "api-0", PodUID: "pod-0", Container: "api", ContainerID: "containerd://target", ImageID: "sha256:image", Node: "node-1", Runtime: "containerd", SeccompMode: "Localhost"}
	controlID := targetID
	controlID.PodName, controlID.PodUID, controlID.ContainerID = "control-0", "control-uid", "containerd://control"
	controlID.SeccompMode = "RuntimeDefault"
	control := &ProbeOutput{Syscall: ProbeSyscall, Result: 0, Errno: 0}
	target := &ProbeOutput{Syscall: ProbeSyscall, Result: -1, Errno: 1}
	fact := Evaluate(target, control, targetID, controlID, now)
	fact.AttemptID = "authority-attempt-1"
	fact.Revocation = "UNKNOWN"
	if fact.Result != Verified || fact.ProbeID != ProbeID || !fact.ValidUntil.After(fact.ObservedAt) {
		t.Fatalf("valid denial/control = %+v", fact)
	}
	fact.Target = targetID
	if _, err := fact.DomainFact("sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatalf("domain verification fact: %v", err)
	}
	target.Result, target.Errno = 0, 0
	if got := Evaluate(target, control, targetID, controlID, now); got.Result != NotVerified {
		t.Fatalf("target success = %s", got.Result)
	}
	if got := Evaluate(&ProbeOutput{Syscall: ProbeSyscall}, control, targetID, controlID, now); got.Result != NotVerified {
		t.Fatalf("fully observed target success = %s", got.Result)
	}
	if got := Evaluate(&ProbeOutput{Syscall: "other"}, control, targetID, controlID, now); got.Result != Unknown {
		t.Fatalf("invalid evidence = %s", got.Result)
	}
	if got := Evaluate(target, nil, targetID, controlID, now); got.Result != Unknown {
		t.Fatalf("missing control = %s", got.Result)
	}
	controlID.ImageID = "sha256:other"
	if got := Evaluate(target, control, targetID, controlID, now); got.Result != Unknown {
		t.Fatalf("unmatched image = %s", got.Result)
	}
	targetID.ContainerID = ""
	if got := Evaluate(target, control, targetID, controlID, now); got.Result != Unknown {
		t.Fatalf("missing runtime identity = %s", got.Result)
	}
}

func TestParseFixedProbeOutput(t *testing.T) {
	for _, tc := range []struct {
		name, line string
		wantErrno  int64
		wantErr    bool
	}{
		{"success", "syscall=getpriority result=0 errno=0 errno_name=Success status=success\n", 0, false},
		{"denied", "syscall=getpriority result=-1 errno=1 errno_name=Operation not permitted status=denied\n", 1, false},
		{"wrong syscall", "syscall=getpid result=1 errno=0 errno_name=Success status=success\n", 0, true},
		{"extra line", "syscall=getpriority result=0 errno=0 errno_name=Success status=success\nnoise\n", 0, true},
		{"json spoof", `{"syscall":"getpriority","result":-1,"errno":1}`, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseProbeOutput(strings.NewReader(tc.line))
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseProbeOutput error=%v", err)
			}
			if err == nil && (got.Syscall != ProbeSyscall || got.Errno != tc.wantErrno) {
				t.Fatalf("parsed result=%+v", got)
			}
		})
	}
}

func TestPersistenceRejectsUnsubstantiatedRevocationAndInconsistentOutcomes(t *testing.T) {
	now := time.Now().UTC()
	f := Fact{AttemptID: "attempt", RequestedBy: "alice", VerifierVersion: "1", VerifierIdentity: "system:serviceaccount:verifier:seccomp-verifier", ProbeID: ProbeID, ObservedAt: now, ValidUntil: now.Add(time.Minute), Result: Unknown, Reason: "test", Revocation: "NOT_REVOKED"}
	if err := f.ValidateForPersistence(); err == nil {
		t.Fatal("unsubstantiated NOT_REVOKED accepted")
	}
	f.Revocation = "UNKNOWN"
	if err := f.ValidateForPersistence(); err != nil {
		t.Fatalf("explicit unknown revocation rejected: %v", err)
	}
}

func TestFactWireIdentityUsesStableLowerCamelJSON(t *testing.T) {
	b, err := json.Marshal(Fact{VerifierIdentity: "system:serviceaccount:verify:seccomp-verifier", Target: Identity{ProposalUID: "proposal", ProfileUID: "profile", PodUID: "pod", ContainerID: "containerd://id"}})
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, field := range []string{`"verifierIdentity"`, `"workloadTarget"`, `"proposalUID"`, `"profileUID"`, `"podUID"`, `"containerID"`} {
		if !strings.Contains(text, field) {
			t.Fatalf("wire fact %s lacks %s", text, field)
		}
	}
	for _, field := range []string{`"ProposalUID"`, `"ProfileUID"`, `"ContainerID"`} {
		if strings.Contains(text, field) {
			t.Fatalf("wire fact contains Go field spelling %s", field)
		}
	}
}
