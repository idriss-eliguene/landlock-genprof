// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package authn contains the transport-only trust contract for an
// Operations Center request. Authentication is intentionally external: a
// trusted reverse proxy authenticates the human and signs the resulting
// identity before forwarding it to this process.
package authn

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	UserHeader      = "X-Operations-Center-User"
	GroupsHeader    = "X-Operations-Center-Groups"
	SignatureHeader = "X-Operations-Center-Signature"
	ProxyHeader     = "X-Operations-Center-Proxy"
	TimestampHeader = "X-Operations-Center-Timestamp"
)

// Identity is request metadata, not a domain or Kubernetes object.
type Identity struct {
	Username string
	Groups   []string
}

// Verifier authenticates the identity assertion made by the trusted proxy.
// The secret is configured out-of-band and is never serialized into a
// request or domain object.
type Policy struct {
	AllowedUsers  map[string]struct{}
	AllowedGroups map[string]struct{}
}

func NewPolicy(users, groups []string) (Policy, error) {
	policy := Policy{AllowedUsers: make(map[string]struct{}), AllowedGroups: make(map[string]struct{})}
	for _, user := range users {
		user = strings.TrimSpace(user)
		if err := validatePrincipal(user, "user"); err != nil {
			return Policy{}, err
		}
		policy.AllowedUsers[user] = struct{}{}
	}
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if err := validatePrincipal(group, "group"); err != nil {
			return Policy{}, err
		}
		policy.AllowedGroups[group] = struct{}{}
	}
	if len(policy.AllowedUsers) == 0 && len(policy.AllowedGroups) == 0 {
		return Policy{}, fmt.Errorf("identity allowlist must contain at least one user or group")
	}
	return policy, nil
}

func (p Policy) Allows(identity Identity) bool {
	if ValidateImpersonationIdentity(identity) != nil {
		return false
	}
	if _, ok := p.AllowedUsers[identity.Username]; ok {
		return true
	}
	for _, group := range identity.Groups {
		if _, ok := p.AllowedGroups[group]; ok {
			return true
		}
	}
	return false
}

// ValidateImpersonationIdentity applies principal-level Kubernetes safety
// rules independently of the configured allowlist. It is used again at the
// client-construction boundary so callers cannot bypass the transport policy
// by constructing an impersonated client directly.
func ValidateImpersonationIdentity(identity Identity) error {
	normalized, err := normalize(identity)
	if err != nil {
		return err
	}
	if strings.HasPrefix(normalized.Username, "system:") || strings.HasPrefix(normalized.Username, "system:serviceaccount:") {
		return fmt.Errorf("Kubernetes system and service-account users are not permitted")
	}
	for _, group := range normalized.Groups {
		if strings.HasPrefix(group, "system:") || group == "system:masters" || group == "system:authenticated" || group == "system:unauthenticated" {
			return fmt.Errorf("Kubernetes system groups are not permitted")
		}
	}
	return nil
}

type Verifier struct {
	secret []byte
	policy Policy
	now    func() time.Time
	maxAge time.Duration
}

func NewVerifier(secret []byte) (*Verifier, error) {
	return NewVerifierWithPolicy(secret, Policy{})
}

func NewVerifierWithPolicy(secret []byte, policy Policy) (*Verifier, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("identity verification secret must be at least 32 bytes")
	}
	return &Verifier{secret: append([]byte(nil), secret...), policy: policy, now: time.Now, maxAge: 5 * time.Minute}, nil
}

func NewVerifierWithPolicyAndClock(secret []byte, policy Policy, now func() time.Time, maxAge time.Duration) (*Verifier, error) {
	if now == nil || maxAge <= 0 {
		return nil, fmt.Errorf("identity verifier clock and acceptance window are required")
	}
	verifier, err := NewVerifierWithPolicy(secret, policy)
	if err != nil {
		return nil, err
	}
	verifier.now = now
	verifier.maxAge = maxAge
	return verifier, nil
}

// FromRequest accepts exactly one signed assertion. Plain client-supplied
// identity headers are never trusted. Duplicate assertion headers fail closed.
func (v *Verifier) FromRequest(r *http.Request) (Identity, error) {
	if v == nil || len(v.secret) == 0 {
		return Identity{}, fmt.Errorf("identity verifier is not configured")
	}
	for _, name := range []string{UserHeader, GroupsHeader, SignatureHeader, ProxyHeader, TimestampHeader} {
		if len(r.Header.Values(name)) != 1 {
			return Identity{}, fmt.Errorf("exactly one %s header is required", name)
		}
	}
	if r.Header.Get(ProxyHeader) != "true" {
		return Identity{}, fmt.Errorf("trusted proxy assertion is required")
	}
	timestamp, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(r.Header.Get(TimestampHeader)))
	if err != nil {
		return Identity{}, fmt.Errorf("identity assertion timestamp is invalid")
	}
	if delta := v.now().Sub(timestamp); delta > v.maxAge || delta < -v.maxAge {
		return Identity{}, fmt.Errorf("identity assertion timestamp is outside acceptance window")
	}
	identity, err := normalize(Identity{Username: r.Header.Get(UserHeader), Groups: strings.Split(r.Header.Get(GroupsHeader), ",")})
	if err != nil {
		return Identity{}, err
	}
	if !v.policy.Allows(identity) {
		return Identity{}, fmt.Errorf("identity is not permitted by the configured allowlist")
	}
	want := signature(v.secret, identity, timestamp)
	got, err := hex.DecodeString(strings.TrimSpace(r.Header.Get(SignatureHeader)))
	if err != nil || !hmac.Equal(got, want) {
		return Identity{}, fmt.Errorf("invalid trusted proxy identity signature")
	}
	return identity, nil
}

func Normalize(identity Identity) (Identity, error) { return normalize(identity) }

func normalize(identity Identity) (Identity, error) {
	identity.Username = strings.TrimSpace(identity.Username)
	if identity.Username == "" || !utf8.ValidString(identity.Username) || strings.ContainsAny(identity.Username, "\r\n") {
		return Identity{}, fmt.Errorf("identity username is invalid")
	}
	groups := make([]string, 0, len(identity.Groups))
	seen := make(map[string]struct{}, len(identity.Groups))
	for _, group := range identity.Groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if !utf8.ValidString(group) {
			return Identity{}, fmt.Errorf("identity group is invalid")
		}
		for _, r := range group {
			if unicode.IsControl(r) || r == ',' {
				return Identity{}, fmt.Errorf("identity group is invalid")
			}
		}
		if _, ok := seen[group]; !ok {
			seen[group] = struct{}{}
			groups = append(groups, group)
		}
	}
	sort.Strings(groups)
	identity.Groups = groups
	return identity, nil
}

func validatePrincipal(value, kind string) error {
	if value == "" || !utf8.ValidString(value) || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("identity %s is invalid", kind)
	}
	if strings.HasPrefix(value, "system:") || strings.HasPrefix(value, "system:serviceaccount:") {
		return fmt.Errorf("identity %s is not permitted", kind)
	}
	if kind == "group" && (value == "system:masters" || value == "system:authenticated" || value == "system:unauthenticated") {
		return fmt.Errorf("identity group is not permitted")
	}
	return nil
}

func signature(secret []byte, identity Identity, timestamp time.Time) []byte {
	canonical := timestamp.UTC().Format(time.RFC3339Nano) + "\n" + identity.Username + "\n" + strings.Join(identity.Groups, "\n")
	h := hmac.New(sha256.New, secret)
	_, _ = h.Write([]byte(canonical))
	return h.Sum(nil)
}
