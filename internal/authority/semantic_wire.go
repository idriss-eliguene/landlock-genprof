package authority

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"unicode/utf8"
)

// SemanticVersion is RFC-0004's four-component numeric version.
type SemanticVersion struct{ parts [4]uint32 }

func ParseSemanticVersion(s string) (SemanticVersion, error) {
	var v SemanticVersion
	parts := [4]uint32{}
	start := 0
	part := 0
	for i := 0; i <= len(s); i++ {
		if i != len(s) && s[i] != '.' {
			continue
		}
		if i == start || (i-start > 1 && s[start] == '0') {
			return v, fmt.Errorf("invalid semantic version")
		}
		n, err := strconv.ParseUint(s[start:i], 10, 32)
		if err != nil {
			return v, fmt.Errorf("invalid semantic version")
		}
		if part >= 4 {
			return v, fmt.Errorf("invalid semantic version")
		}
		parts[part] = uint32(n)
		part++
		start = i + 1
	}
	if start != len(s)+1 || part != 4 {
		return v, fmt.Errorf("invalid semantic version")
	}
	// The zero value is ambiguous while parsing, so count separators directly.
	count := bytes.Count([]byte(s), []byte("."))
	if count != 3 {
		return v, fmt.Errorf("invalid semantic version")
	}
	v.parts = parts
	return v, nil
}

func (v SemanticVersion) Compare(other SemanticVersion) int {
	for i := 0; i < 4; i++ {
		if v.parts[i] < other.parts[i] {
			return -1
		}
		if v.parts[i] > other.parts[i] {
			return 1
		}
	}
	return 0
}
func (v SemanticVersion) String() string {
	return fmt.Sprintf("%d.%d.%d.%d", v.parts[0], v.parts[1], v.parts[2], v.parts[3])
}

type VersionRange struct{ lower, upper SemanticVersion }

func NewVersionRange(lower, upper SemanticVersion) (VersionRange, error) {
	if lower.Compare(upper) > 0 {
		return VersionRange{}, fmt.Errorf("invalid version range")
	}
	return VersionRange{lower: lower, upper: upper}, nil
}
func (r VersionRange) Matches(v SemanticVersion) bool {
	return r.lower.Compare(v) <= 0 && v.Compare(r.upper) <= 0
}

// StrictObject validates JSON syntax, UTF-8, duplicate keys at every depth,
// and trailing values before object-specific decoding.
func StrictObject(raw []byte) (map[string]json.RawMessage, error) {
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("invalid UTF-8")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var value any
	if err := decodeUnique(dec, &value); err != nil {
		return nil, err
	}
	var extra any
	if dec.Decode(&extra) == nil {
		return nil, fmt.Errorf("trailing JSON")
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("object required")
	}
	out := make(map[string]json.RawMessage, len(obj))
	for k, v := range obj {
		b, _ := json.Marshal(v)
		out[k] = b
	}
	return out, nil
}

func ValidateV1Envelope(fields map[string]json.RawMessage) error {
	for _, k := range []string{"schemaId", "schemaVersion", "kind", "id", "version"} {
		raw, ok := fields[k]
		if !ok {
			return fmt.Errorf("missing envelope field %q", k)
		}
		var s string
		if json.Unmarshal(raw, &s) != nil || s == "" {
			return fmt.Errorf("invalid envelope field %q", k)
		}
	}
	return nil
}

func decodeUnique(dec *json.Decoder, out *any) error {
	t, err := dec.Token()
	if err != nil {
		return err
	}
	switch x := t.(type) {
	case json.Delim:
		if x == '{' {
			m := map[string]any{}
			seen := map[string]bool{}
			for dec.More() {
				kt, e := dec.Token()
				if e != nil {
					return e
				}
				k, ok := kt.(string)
				if !ok || seen[k] {
					return fmt.Errorf("duplicate/invalid key")
				}
				seen[k] = true
				var v any
				if e = decodeUnique(dec, &v); e != nil {
					return e
				}
				m[k] = v
			}
			if _, e := dec.Token(); e != nil {
				return e
			}
			*out = m
			return nil
		}
		if x == '[' {
			a := []any{}
			for dec.More() {
				var v any
				if e := decodeUnique(dec, &v); e != nil {
					return e
				}
				a = append(a, v)
			}
			if _, e := dec.Token(); e != nil {
				return e
			}
			*out = a
			return nil
		}
	}
	*out = t
	return nil
}
