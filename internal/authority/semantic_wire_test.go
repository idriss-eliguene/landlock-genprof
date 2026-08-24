package authority

import "testing"

func TestSemanticVersionAndRange(t *testing.T) {
	v, err := ParseSemanticVersion("1.10.0.0")
	if err != nil {
		t.Fatal(err)
	}
	w, _ := ParseSemanticVersion("1.2.0.0")
	if v.Compare(w) <= 0 {
		t.Fatal("numeric comparison")
	}
	for _, s := range []string{"01.2.3.4", "1.2.3", "1.2.3.4.5", "v1.2.3.4", "1.2.3.4-alpha", "4294967296.0.0.0"} {
		if _, err := ParseSemanticVersion(s); err == nil {
			t.Fatalf("accepted %s", s)
		}
	}
	l, _ := ParseSemanticVersion("1.0.0.0")
	u, _ := ParseSemanticVersion("2.0.0.0")
	r, _ := NewVersionRange(l, u)
	if !r.Matches(l) || !r.Matches(u) {
		t.Fatal("inclusive range")
	}
}
func TestStrictObjectRejectsDuplicatesAndNestedEnvelope(t *testing.T) {
	for _, raw := range []string{`{"schemaId":"x","schemaId":"y"}`, `{"x":{"a":1,"a":2}}`, `{"schema":{"id":"x","version":"1"}}`, `{"x":1} {"y":2}`} {
		fields, err := StrictObject([]byte(raw))
		if err == nil && ValidateV1Envelope(fields) == nil {
			t.Fatalf("accepted invalid %s", raw)
		}
	}
}
