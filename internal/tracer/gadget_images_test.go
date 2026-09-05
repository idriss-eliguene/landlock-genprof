// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

//go:build linux

package tracer

import (
	"strings"
	"testing"
)

// TestGadgetImagesArePinned verifies that every runtime OCI gadget image
// reference used by the tracer is an immutable digest-pinned reference and
// does not carry a mutable floating tag (:latest, :main, etc.). This
// prevents silent version skew between the IG SDK, the deployed Gadget
// operator, and the OCI gadget images that caused zero-timestamp failures
// in CI (run 31887044669).
//
// To update digests after an intentional SDK/gadget upgrade:
//  1. bump github.com/inspektor-gadget/inspektor-gadget in go.mod,
//  2. bump IG_VERSION in test/e2e/install-gadget.sh to match,
//  3. verify builder.version annotations on new :latest digests,
//  4. replace the @sha256:... constants in internal/tracer/trace_linux.go,
//  5. update this test to accept the new digests.
func TestGadgetImagesArePinned(t *testing.T) {
	if GadgetCompatibilityVersion == "" {
		t.Fatal("GadgetCompatibilityVersion must be non-empty")
	}
	if GadgetCompatibilityVersion != "v0.55.1" {
		t.Fatalf("unexpected GadgetCompatibilityVersion=%q, want %q", GadgetCompatibilityVersion, "v0.55.1")
	}

	expected := map[string]string{
		"trace_open":         "trace_open@sha256:4f76f168da0ae70f33e9ceae6703eb6131d5270f32dd732f3897e1ba06d94a3d",
		"trace_exec":         "trace_exec@sha256:d0488d06fc9c9f747b6b56d8b1c9fc03dfbd931a01c2296b4f7164821f0b1792",
		"trace_tcp":          "trace_tcp@sha256:5808d2dd41f872a405ebf5b4ccb005974162b9f1444e4191f2df8cbce9abeef3",
		"trace_bind":         "trace_bind@sha256:643eb030aaae9117e7407a83761c3fd70673290f489f1a3e97092d698155918e",
		"trace_capabilities": "trace_capabilities@sha256:1b9cf532ed68da08861ca9b4c7c05e9829dd3bb3ec1f31d327eb5acf236c4e6e",
		"advise_seccomp":     "advise_seccomp@sha256:6effe59329500c313aea5f0053a9a06982a30e6a4b0d985cfe10a0612f5ddd59",
	}

	if len(requiredGadgetImages) != 6 {
		t.Fatalf("requiredGadgetImages must contain exactly 6 gadgets, got %d", len(requiredGadgetImages))
	}
	for key, want := range expected {
		got, ok := requiredGadgetImages[key]
		if !ok {
			t.Fatalf("requiredGadgetImages missing key %q", key)
		}
		if got != want {
			t.Fatalf("requiredGadgetImages[%q]=%q, want %q", key, got, want)
		}
	}

	for name, ref := range requiredGadgetImages {
		t.Run(name, func(t *testing.T) {
			if strings.HasSuffix(ref, ":latest") {
				t.Errorf("gadget image %s=%q uses mutable :latest tag; pin to an immutable @sha256:<digest>", name, ref)
			}
			if strings.HasSuffix(ref, ":main") {
				t.Errorf("gadget image %s=%q uses mutable :main tag; pin to an immutable @sha256:<digest>", name, ref)
			}
			if !strings.Contains(ref, "@sha256:") {
				t.Errorf("gadget image %s=%q is not digest-pinned; expected @sha256:<64-hex-chars>", name, ref)
			}
			idx := strings.Index(ref, "@sha256:")
			if idx < 0 {
				return
			}
			digest := ref[idx+len("@sha256:"):]
			if len(digest) != 64 {
				t.Errorf("gadget image %s=%q has malformed digest (want 64 hex chars, got %d)", name, ref, len(digest))
			}
			for _, c := range digest {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
					t.Errorf("gadget image %s=%q digest contains non-hex character %q", name, ref, c)
					break
				}
			}
		})
	}
}

func TestTraceTCPConnectContract(t *testing.T) {
	if _, ok := requiredGadgetImages["trace_tcpconnect"]; ok {
		t.Fatal("requiredGadgetImages must not include trace_tcpconnect")
	}
	if _, ok := requiredGadgetImages["trace_tcp"]; !ok {
		t.Fatal("requiredGadgetImages must include trace_tcp")
	}
	if got := requiredGadgetImages["trace_tcp"]; got != traceTCPImage {
		t.Fatalf("requiredGadgetImages[trace_tcp]=%q, want %q", got, traceTCPImage)
	}
	if got := traceTCPBackendKind; got != "trace_tcp" {
		t.Fatalf("traceTCPBackendKind=%q, want %q", got, "trace_tcp")
	}
	if got := traceTCPConnectOnlyParam; got != "operator.oci.ebpf.connect-only" {
		t.Fatalf("traceTCPConnectOnlyParam=%q, want %q", got, "operator.oci.ebpf.connect-only")
	}
	if got := traceTCPDstPortField; got != "dst.port" {
		t.Fatalf("traceTCPDstPortField=%q, want %q", got, "dst.port")
	}
	if got := traceTCPCommField; got != "proc.comm" {
		t.Fatalf("traceTCPCommField=%q, want %q", got, "proc.comm")
	}
	if got := traceTCPErrorRawField; got != "error_raw" {
		t.Fatalf("traceTCPErrorRawField=%q, want %q", got, "error_raw")
	}
	if got := traceTCPTimestampField; got != "timestamp_raw" {
		t.Fatalf("traceTCPTimestampField=%q, want %q", got, "timestamp_raw")
	}
}
