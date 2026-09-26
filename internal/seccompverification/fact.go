// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package seccompverification models the deliberately narrow getpriority
// runtime experiment. It does not claim general Seccomp correctness.
package seccompverification

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/authority"
)

const (
	ProbeID                   = "linux-seccomp-getpriority-v1"
	ProbePath                 = "/seccomp-verifier-probe"
	ProbeSyscall              = "getpriority"
	VerifierID                = "landlock-genprof-seccomp-probe"
	VerifierServiceAccount    = "seccomp-verifier"
	VerifierAPIServiceAccount = "landlock-genprof-seccomp-verifier"
)

var probeOutputPattern = regexp.MustCompile(`^syscall=getpriority result=(-?[0-9]+) errno=([0-9]+) errno_name=(Success|Operation not permitted|Unknown error) status=(success|denied)$`)

type Result string

const (
	Verified    Result = "VERIFIED"
	NotVerified Result = "NOT_VERIFIED"
	Unknown     Result = "UNKNOWN"
)

// ProbeOutput is parsed only from the fixed helper's machine-readable output.
// Exit status alone is insufficient because EPERM is the property under test.
type ProbeOutput struct {
	Syscall string `json:"syscall"`
	Result  int64  `json:"result"`
	Errno   int64  `json:"errno"`
}

// ParseProbeOutput accepts exactly the single-line format emitted by the
// repository's seccomp-probe helper. It intentionally does not accept JSON,
// shell output, extra lines, or an arbitrary command's stdout.
func ParseProbeOutput(r io.Reader) (ProbeOutput, error) {
	scanner := bufio.NewScanner(io.LimitReader(r, 4097))
	if !scanner.Scan() {
		return ProbeOutput{}, fmt.Errorf("probe produced no result")
	}
	line := strings.TrimSpace(scanner.Text())
	if scanner.Scan() || scanner.Err() != nil {
		return ProbeOutput{}, fmt.Errorf("probe output must be exactly one line")
	}
	match := probeOutputPattern.FindStringSubmatch(line)
	if match == nil {
		return ProbeOutput{}, fmt.Errorf("malformed probe output")
	}
	result, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return ProbeOutput{}, fmt.Errorf("invalid syscall result")
	}
	errnoValue, err := strconv.ParseInt(match[2], 10, 32)
	if err != nil {
		return ProbeOutput{}, fmt.Errorf("invalid syscall errno")
	}
	if (errnoValue == 0) != (match[4] == "success") {
		return ProbeOutput{}, fmt.Errorf("probe status contradicts errno")
	}
	return ProbeOutput{Syscall: ProbeSyscall, Result: result, Errno: errnoValue}, nil
}

type Identity struct {
	ProposalUID     string `json:"proposalUID"`
	CandidateDigest string `json:"candidateDigest"`
	Namespace       string `json:"namespace"`
	ProfileName     string `json:"profileName"`
	ProfileUID      string `json:"profileUID"`
	ProfileDigest   string `json:"profileDigest"`
	WorkloadUID     string `json:"workloadUID"`
	Workload        string `json:"workload"`
	PodName         string `json:"podName"`
	PodUID          string `json:"podUID"`
	Container       string `json:"container"`
	ContainerID     string `json:"containerID"`
	ImageID         string `json:"imageID"`
	Node            string `json:"node"`
	Runtime         string `json:"runtime"`
	SeccompMode     string `json:"seccompMode"`
}

type ProfileMaterialization struct {
	State         string    `json:"state"`
	Source        string    `json:"source"`
	ProfileUID    string    `json:"profileUID"`
	ContentDigest string    `json:"contentDigest"`
	LocalhostPath string    `json:"localhostPath"`
	Node          string    `json:"node"`
	ObservedAt    time.Time `json:"observedAt"`
	Limitation    string    `json:"limitation"`
}

type TargetConfiguration struct {
	State         string    `json:"state"`
	PodUID        string    `json:"podUID"`
	ContainerID   string    `json:"containerID"`
	Node          string    `json:"node"`
	LocalhostPath string    `json:"localhostPath"`
	ObservedAt    time.Time `json:"observedAt"`
	Limitation    string    `json:"limitation"`
}

func (i Identity) complete() bool {
	return i.ProposalUID != "" && i.CandidateDigest != "" && i.Namespace != "" &&
		i.ProfileName != "" && i.ProfileUID != "" && i.ProfileDigest != "" &&
		i.WorkloadUID != "" && i.Workload != "" && i.PodName != "" && i.PodUID != "" &&
		i.Container != "" && i.ContainerID != "" && i.ImageID != "" && i.Node != "" && i.Runtime != ""
}

type Fact struct {
	AttemptID           string                 `json:"attemptID"`
	RequestedBy         string                 `json:"requestedBy"`
	VerifierIdentity    string                 `json:"verifierIdentity"`
	VerifierImage       string                 `json:"verifierImage"`
	VerifierVersion     string                 `json:"verifierVersion"`
	ProbeID             string                 `json:"probeID"`
	ObservedAt          time.Time              `json:"observedAt"`
	ValidUntil          time.Time              `json:"validUntil"`
	Result              Result                 `json:"result"`
	Reason              string                 `json:"reason,omitempty"`
	Target              Identity               `json:"workloadTarget"`
	Materialization     ProfileMaterialization `json:"profileMaterialization"`
	TargetConfiguration TargetConfiguration    `json:"targetConfiguration"`
	Twin                Identity               `json:"twin"`
	Control             Identity               `json:"control"`
	TargetOutput        ProbeOutput            `json:"targetOutput"`
	ControlOutput       ProbeOutput            `json:"controlOutput"`
	Experiment          Experiment             `json:"experiment"`
	Revocation          string                 `json:"revocation"`
}

func NewAttemptID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func (f Fact) ValidateForPersistence() error {
	if _, err := authority.NewResolutionAttemptIdentity(f.AttemptID); err != nil || f.ProbeID != ProbeID || f.VerifierVersion == "" || f.VerifierIdentity == "" || f.ObservedAt.IsZero() || (f.Result != Verified && f.Result != NotVerified && f.Result != Unknown) {
		return fmt.Errorf("invalid Seccomp verification fact envelope")
	}
	if f.ValidUntil.IsZero() || f.ValidUntil.Before(f.ObservedAt) {
		return fmt.Errorf("invalid Seccomp verification validity")
	}
	if f.Result == Unknown && strings.TrimSpace(f.Reason) == "" {
		return fmt.Errorf("unknown Seccomp verification fact requires a reason")
	}
	if f.Revocation != "UNKNOWN" {
		return fmt.Errorf("this verifier has no revocation source; revocation must remain UNKNOWN")
	}
	if f.Result != Unknown && (f.Target.PodUID == "" || f.Target.ContainerID == "" || f.Target.ProfileUID == "" || f.Target.CandidateDigest == "" || f.Twin.PodUID == "" || f.Control.PodUID == "" || f.Experiment.Twin.ExitCode == nil || f.Experiment.Control.ExitCode == nil) {
		return fmt.Errorf("conclusive Seccomp verification fact lacks target binding")
	}
	if f.Result != Unknown {
		if f.VerifierIdentity != "system:serviceaccount:"+f.Twin.Namespace+":"+VerifierAPIServiceAccount {
			return fmt.Errorf("conclusive fact verifier identity differs from the verifier Pod ServiceAccount")
		}
		if !f.Target.complete() || !f.Twin.complete() || !f.Control.complete() ||
			f.Twin.Namespace != f.Control.Namespace || f.Twin.CandidateDigest != f.Target.CandidateDigest || f.Control.CandidateDigest != f.Target.CandidateDigest ||
			f.Twin.ProfileUID != f.Target.ProfileUID || f.Control.ProfileUID != f.Target.ProfileUID || f.Twin.ProfileDigest != f.Target.ProfileDigest || f.Control.ProfileDigest != f.Target.ProfileDigest ||
			f.Twin.SeccompMode != "Localhost" || f.Control.SeccompMode != "RuntimeDefault" ||
			f.Twin.Node != f.Control.Node || f.Twin.Runtime != f.Control.Runtime || f.Twin.ImageID != f.Control.ImageID {
			return fmt.Errorf("conclusive Seccomp verification identities are incomplete or mismatched")
		}
		if err := ValidateProbeImage(f.VerifierImage); err != nil || f.VerifierImage != f.Experiment.VerifierImage {
			return fmt.Errorf("digest-pinned verifier image reference is unavailable or mismatched")
		}
		if f.Materialization.State != "MATERIALIZED" || f.Materialization.ProfileUID != f.Target.ProfileUID || f.Materialization.ContentDigest != f.Target.ProfileDigest || f.Materialization.Node != f.Target.Node || f.TargetConfiguration.State != "CONFIGURED_NOT_RUNTIME_VERIFIED" || f.TargetConfiguration.PodUID != f.Target.PodUID || f.TargetConfiguration.ContainerID != f.Target.ContainerID {
			return fmt.Errorf("conclusive experiment lacks distinct materialization and target-configuration evidence")
		}
		imageDigest := strings.SplitN(f.VerifierImage, "@", 2)[1]
		if _, err := f.DomainFact(imageDigest); err != nil {
			return fmt.Errorf("resolving observation in authority domain: %w", err)
		}
	}
	if f.Result == Verified && (*f.Experiment.Twin.ExitCode != ExitEPERM || *f.Experiment.Control.ExitCode != ExitSuccess) {
		return fmt.Errorf("VERIFIED requires target EPERM and successful RuntimeDefault control")
	}
	if f.Result == NotVerified && (*f.Experiment.Twin.ExitCode != ExitSuccess || *f.Experiment.Control.ExitCode != ExitSuccess) {
		return fmt.Errorf("NOT_VERIFIED requires fully observed successful probes")
	}
	return nil
}

// Evaluate never interprets absent observations as a successful negative.
// Identity revalidation is performed by the caller before this function.
func Evaluate(target, control *ProbeOutput, targetID, controlID Identity, now time.Time) Fact {
	f := Fact{VerifierVersion: "1", VerifierIdentity: "system:serviceaccount:" + targetID.Namespace + ":" + VerifierAPIServiceAccount, ProbeID: ProbeID, ObservedAt: now.UTC(), ValidUntil: now.UTC().Add(5 * time.Minute), Result: Unknown, Twin: targetID, Control: controlID}
	if target != nil {
		f.TargetOutput = *target
	}
	if control != nil {
		f.ControlOutput = *control
	}
	if target == nil || control == nil {
		f.Reason = "target or RuntimeDefault control probe unavailable"
		return f
	}
	if !targetID.complete() || !controlID.complete() {
		f.Reason = "runtime identity incomplete"
		return f
	}
	if targetID.Namespace != controlID.Namespace || targetID.Container != controlID.Container || targetID.ImageID != controlID.ImageID || targetID.Node != controlID.Node || targetID.Runtime != controlID.Runtime || targetID.SeccompMode != "Localhost" || controlID.SeccompMode != "RuntimeDefault" {
		f.Reason = "target and control are not a matched runtime pair"
		return f
	}
	if target.Syscall != ProbeSyscall || control.Syscall != ProbeSyscall || control.Result != 0 || control.Errno != 0 {
		f.Reason = "probe output invalid or RuntimeDefault control did not succeed"
		return f
	}
	if target.Result == -1 && target.Errno == 1 {
		f.Result = Verified
		return f
	}
	f.Result = NotVerified
	f.Reason = "target did not return EPERM for getpriority"
	return f
}

// DomainFact validates the persisted result through the project's existing
// verification-fact domain. Diagnostic runtime IDs remain in the serialized
// fact, not in SecurityContextIdentity.
func (f Fact) DomainFact(verifierDigest string) (authority.ResolvedVerificationFact, error) {
	if f.Result == Unknown {
		return authority.ResolvedVerificationFact{}, fmt.Errorf("unknown result is not an affirmative verification fact")
	}
	result := authority.VerificationFactFailed
	if f.Result == Verified {
		result = authority.VerificationFactVerified
	}
	return authority.NewSeccompRuntimeEvidenceFact(authority.SeccompRuntimeEvidence{AttemptID: f.AttemptID, Version: f.VerifierVersion, Digest: verifierDigest, Subject: f.Target.ProposalUID + "/" + f.Target.CandidateDigest, ImageIdentity: f.Target.ImageID, WorkloadIdentity: f.Target.WorkloadUID + "/" + f.Target.PodUID, Runtime: f.Target.Runtime, ObservedAt: f.ObservedAt, ValidUntil: f.ValidUntil, Result: result})
}
