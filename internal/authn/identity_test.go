package authn

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func signedRequest(t *testing.T, user string, groups string, secret []byte) *http.Request {
	return signedRequestAt(t, user, groups, secret, time.Now().UTC())
}

func signedRequestAt(t *testing.T, user string, groups string, secret []byte, timestamp time.Time) *http.Request {
	t.Helper()
	r := httptest.NewRequest("GET", "http://127.0.0.1/api/v08/capabilities", nil)
	r.Header.Set(UserHeader, user)
	r.Header.Set(GroupsHeader, groups)
	r.Header.Set(ProxyHeader, "true")
	r.Header.Set(TimestampHeader, timestamp.UTC().Format(time.RFC3339Nano))
	normalized, err := Normalize(Identity{Username: user, Groups: strings.Split(groups, ",")})
	if err != nil {
		t.Fatal(err)
	}
	h := hmac.New(sha256.New, secret)
	_, _ = h.Write([]byte(timestamp.UTC().Format(time.RFC3339Nano) + "\n" + normalized.Username + "\n" + joinGroups(normalized.Groups)))
	r.Header.Set(SignatureHeader, hex.EncodeToString(h.Sum(nil)))
	return r
}

func TestSignAssertionHeadersProducesAVerifierAcceptedRequest(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	policy, err := NewPolicy([]string{"alice@company"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewVerifierWithPolicy(secret, policy)
	if err != nil {
		t.Fatal(err)
	}
	headers, err := SignAssertionHeaders(secret, Identity{Username: "alice@company", Groups: []string{"team-a"}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1/api/v08/environment", nil)
	for name, values := range headers {
		for _, v := range values {
			r.Header.Add(name, v)
		}
	}
	identity, err := verifier.FromRequest(r)
	if err != nil {
		t.Fatalf("SignAssertionHeaders output was rejected by the real Verifier: %v", err)
	}
	if identity.Username != "alice@company" {
		t.Fatalf("unexpected identity: %#v", identity)
	}

	stale, err := SignAssertionHeaders(secret, Identity{Username: "alice@company"}, time.Now().Add(-10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	staleRequest := httptest.NewRequest("GET", "http://127.0.0.1/api/v08/environment", nil)
	for name, values := range stale {
		for _, v := range values {
			staleRequest.Header.Add(name, v)
		}
	}
	if _, err := verifier.FromRequest(staleRequest); err == nil {
		t.Fatal("expected a 10-minute-old assertion to be rejected as stale")
	}
}

func joinGroups(groups []string) string {
	if len(groups) == 0 {
		return ""
	}
	result := groups[0]
	for _, group := range groups[1:] {
		result += "\n" + group
	}
	return result
}

func TestVerifierRequiresSignedTrustedProxyAssertion(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	policy, err := NewPolicy([]string{"alice@company"}, []string{"team-a", "team-operators"})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewVerifierWithPolicy(secret, policy)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := verifier.FromRequest(signedRequest(t, "alice@company", "team-a,team-operators", secret))
	if err != nil {
		t.Fatal(err)
	}
	if identity.Username != "alice@company" || len(identity.Groups) != 2 || identity.Groups[0] != "team-a" {
		t.Fatalf("unexpected identity: %#v", identity)
	}

	for name, mutate := range map[string]func(*http.Request){
		"missing proxy marker": func(r *http.Request) { r.Header.Del(ProxyHeader) },
		"bad signature":        func(r *http.Request) { r.Header.Set(SignatureHeader, "00") },
		"duplicate user":       func(r *http.Request) { r.Header.Add(UserHeader, "mallory") },
	} {
		t.Run(name, func(t *testing.T) {
			r := signedRequest(t, "alice@company", "team-a,team-operators", secret)
			mutate(r)
			if _, err := verifier.FromRequest(r); err == nil {
				t.Fatal("expected fail-closed identity rejection")
			}
		})
	}
}

func TestVerifierRejectsUnapprovedAndPrivilegedIdentities(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	policy, err := NewPolicy([]string{"alice@company"}, []string{"team-a"})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewVerifierWithPolicy(secret, policy)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, user, groups string }{
		{"system masters", "alice@company", "system:masters"},
		{"service account", "system:serviceaccount:team-a:runner", "team-a"},
		{"system group", "alice@company", "system:authenticated"},
		{"unapproved user", "mallory@company", "other-team"},
		{"unapproved group", "mallory@company", "other-team"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := verifier.FromRequest(signedRequest(t, tc.user, tc.groups, secret)); err == nil {
				t.Fatal("expected privileged or unapproved identity to fail")
			}
		})
	}
}

func TestVerifierEnforcesFreshTimestampAndUTF8(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	policy, err := NewPolicy([]string{"alice@company"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	verifier, err := NewVerifierWithPolicyAndClock(secret, policy, func() time.Time { return now }, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		timestamp time.Time
	}{
		{"stale", now.Add(-2 * time.Minute)},
		{"future", now.Add(2 * time.Minute)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := verifier.FromRequest(signedRequestAt(t, "alice@company", "", secret, tc.timestamp)); err == nil {
				t.Fatal("expected timestamp rejection")
			}
		})
	}
	valid := signedRequestAt(t, "alice@company", "", secret, now)
	if _, err := verifier.FromRequest(valid); err != nil {
		t.Fatalf("fresh assertion rejected: %v", err)
	}
	if _, err := Normalize(Identity{Username: string([]byte{'a', 0xff})}); err == nil {
		t.Fatal("expected invalid UTF-8 rejection")
	}
	duplicate := signedRequestAt(t, "alice@company", "", secret, now)
	duplicate.Header.Add(TimestampHeader, duplicate.Header.Get(TimestampHeader))
	if _, err := verifier.FromRequest(duplicate); err == nil {
		t.Fatal("expected duplicate timestamp rejection")
	}
}

func TestVerifierHasNoReplayCache(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	policy, err := NewPolicy([]string{"alice@company"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	verifier, err := NewVerifierWithPolicyAndClock(secret, policy, func() time.Time { return now }, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	request := signedRequestAt(t, "alice@company", "", secret, now)
	if _, err := verifier.FromRequest(request); err != nil {
		t.Fatalf("first assertion rejected: %v", err)
	}
	if _, err := verifier.FromRequest(request); err != nil {
		t.Fatalf("duplicate assertion unexpectedly rejected: %v", err)
	}
	independent, err := NewVerifierWithPolicyAndClock(secret, policy, func() time.Time { return now }, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := independent.FromRequest(request); err != nil {
		t.Fatalf("duplicate assertion rejected by independent verifier: %v", err)
	}
}

func TestPolicyRejectsPrivilegedConfiguredPrincipals(t *testing.T) {
	for _, principal := range []string{"system:masters", "system:authenticated", "system:serviceaccount:team-a:runner"} {
		if _, err := NewPolicy([]string{principal}, nil); err == nil {
			t.Fatalf("configured privileged user %q was accepted", principal)
		}
		if _, err := NewPolicy(nil, []string{principal}); err == nil {
			t.Fatalf("configured privileged group %q was accepted", principal)
		}
	}
}
