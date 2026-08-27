package main

import (
	"os"
	"strings"
	"testing"
)

func TestPodlockSPOLaunchesObservedStaticProbe(t *testing.T) {
	script, err := os.ReadFile("../../test/e2e/podlock-spo-golden.sh")
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)
	if strings.Contains(text, `command: ["sh", "-c"`) {
		t.Fatal("pairwise workload must not bootstrap through sh -c")
	}
	if !strings.Contains(text, `command: ["/probe/fsprobe", "--loop"]`) {
		t.Fatal("pairwise workload must directly execute the observed probe")
	}
	if !strings.Contains(text, `SECCOMP_PROBE=/probe/seccomp-probe`) {
		t.Fatal("seccomp probe must remain in the observed /probe executable domain")
	}
}
