package proposal

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
)

const (
	CandidateV2Version               = "candidate-v2"
	CandidateV2ScopeContainer        = "CONTAINER"
	CandidateV2ArtifactContainerCaps = "CONTAINER_CAPABILITIES"
	candidateV2DomainSeparator       = CandidateV2Version
)

// CandidateV2 is the approval-bound, container-capability candidate domain.
// Provenance and evidence qualification deliberately do not belong here.
type CandidateV2 struct {
	Version  string     `json:"version"`
	Subject  SubjectV2  `json:"subject"`
	Artifact ArtifactV2 `json:"artifact"`
}

type SubjectV2 struct {
	Scope         string `json:"scope"`
	Target        string `json:"target"`
	Container     string `json:"container"`
	ImageIdentity string `json:"imageIdentity"`
}

type ArtifactV2 struct {
	Type                  string                  `json:"type"`
	ContainerCapabilities ContainerCapabilitiesV2 `json:"containerCapabilities"`
}

type ContainerCapabilitiesV2 struct {
	Drop []string `json:"drop"`
	Add  []string `json:"add"`
}

var linuxCapabilityNames = map[string]struct{}{
	"CAP_AUDIT_CONTROL": {}, "CAP_AUDIT_READ": {}, "CAP_AUDIT_WRITE": {},
	"CAP_BLOCK_SUSPEND": {}, "CAP_BPF": {}, "CAP_CHECKPOINT_RESTORE": {},
	"CAP_CHOWN": {}, "CAP_DAC_OVERRIDE": {}, "CAP_DAC_READ_SEARCH": {},
	"CAP_FOWNER": {}, "CAP_FSETID": {}, "CAP_IPC_LOCK": {}, "CAP_IPC_OWNER": {},
	"CAP_KILL": {}, "CAP_LEASE": {}, "CAP_LINUX_IMMUTABLE": {}, "CAP_MAC_ADMIN": {},
	"CAP_MAC_OVERRIDE": {}, "CAP_MKNOD": {}, "CAP_NET_ADMIN": {},
	"CAP_NET_BIND_SERVICE": {}, "CAP_NET_BROADCAST": {}, "CAP_NET_RAW": {},
	"CAP_PERFMON": {}, "CAP_SETFCAP": {}, "CAP_SETGID": {}, "CAP_SETPCAP": {},
	"CAP_SETUID": {}, "CAP_SYSLOG": {}, "CAP_SYS_ADMIN": {}, "CAP_SYS_BOOT": {},
	"CAP_SYS_CHROOT": {}, "CAP_SYS_MODULE": {}, "CAP_SYS_NICE": {},
	"CAP_SYS_PACCT": {}, "CAP_SYS_PTRACE": {}, "CAP_SYS_RAWIO": {},
	"CAP_SYS_RESOURCE": {}, "CAP_SYS_TIME": {}, "CAP_SYS_TTY_CONFIG": {},
	"CAP_WAKE_ALARM": {},
}

func (c CandidateV2) Validate() error {
	if c.Version != CandidateV2Version {
		return fmt.Errorf("candidate-v2: unsupported version %q", c.Version)
	}
	if err := c.Subject.validate(); err != nil {
		return err
	}
	if err := c.Artifact.validate(); err != nil {
		return err
	}
	return nil
}

func (s SubjectV2) validate() error {
	if s.Scope != CandidateV2ScopeContainer {
		return fmt.Errorf("candidate-v2: unsupported subject scope %q", s.Scope)
	}
	if s.Target == "" || s.Container == "" || s.ImageIdentity == "" {
		return fmt.Errorf("candidate-v2: subject identity fields are required")
	}
	return nil
}

func (a ArtifactV2) validate() error {
	if a.Type != CandidateV2ArtifactContainerCaps {
		return fmt.Errorf("candidate-v2: unsupported artifact type %q", a.Type)
	}
	_, err := normalizeCapabilities(a.ContainerCapabilities)
	return err
}

func normalizeCapabilities(c ContainerCapabilitiesV2) (ContainerCapabilitiesV2, error) {
	if len(c.Drop) != 1 || c.Drop[0] != "ALL" {
		return ContainerCapabilitiesV2{}, fmt.Errorf("candidate-v2: drop must be exactly [ALL]")
	}

	seen := make(map[string]struct{}, len(c.Add))
	add := make([]string, 0, len(c.Add))
	for _, name := range c.Add {
		if _, ok := linuxCapabilityNames[name]; !ok {
			return ContainerCapabilitiesV2{}, fmt.Errorf("candidate-v2: unknown capability %q", name)
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		add = append(add, name)
	}
	sort.Strings(add)
	return ContainerCapabilitiesV2{Drop: []string{"ALL"}, Add: add}, nil
}

// CanonicalBytes returns the frozen candidate-v2 binary encoding.
// Every string is uint32-big-endian length-prefixed UTF-8. The domain
// separator is encoded first, followed by the explicit Version field.
func (c CandidateV2) CanonicalBytes() ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	caps, err := normalizeCapabilities(c.Artifact.ContainerCapabilities)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	putString := func(value string) error {
		length, err := checkedUint32Length(len(value))
		if err != nil {
			return fmt.Errorf("candidate-v2: string too long")
		}
		if err := binary.Write(&b, binary.BigEndian, length); err != nil {
			return err
		}
		_, err = b.WriteString(value)
		return err
	}
	putList := func(values []string) error {
		length, err := checkedUint32Length(len(values))
		if err != nil {
			return fmt.Errorf("candidate-v2: list too long")
		}
		if err := binary.Write(&b, binary.BigEndian, length); err != nil {
			return err
		}
		for _, value := range values {
			if err := putString(value); err != nil {
				return err
			}
		}
		return nil
	}
	for _, value := range []string{candidateV2DomainSeparator, c.Version, c.Subject.Scope, c.Subject.Target, c.Subject.Container, c.Subject.ImageIdentity, c.Artifact.Type} {
		if err := putString(value); err != nil {
			return nil, err
		}
	}
	if err := putList(caps.Drop); err != nil {
		return nil, err
	}
	if err := putList(caps.Add); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func CandidateV2CanonicalBytes(c CandidateV2) ([]byte, error) { return c.CanonicalBytes() }

func CandidateDigestV2(c CandidateV2) (string, error) {
	b, err := c.CanonicalBytes()
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:]), nil
}
