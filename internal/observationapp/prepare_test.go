package observationapp

import (
	"testing"
)

func TestSourcePreservesSupportedSourceKinds(t *testing.T) {
	for _, kind := range []string{"filesystem", "exec", "networkConnect", "networkBind", "capabilities"} {
		if _, err := Source(kind); err != nil {
			t.Fatalf("Source(%q) returned error: %v", kind, err)
		}
	}
	if _, err := Source("unsupported"); err == nil {
		t.Fatal("unsupported source accepted")
	}
}
