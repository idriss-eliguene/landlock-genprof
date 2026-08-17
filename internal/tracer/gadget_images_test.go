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
	if GadgetCompatibilityVersion != "v0.55.0" {
		t.Fatalf("unexpected GadgetCompatibilityVersion=%q, want %q", GadgetCompatibilityVersion, "v0.55.0")
	}

	expected := map[string]string{
		"trace_open":         "trace_open@sha256:53cacce4be386be3fa6dd3552f8b91abdd1ac574a2d829bf9b3dd2105de70da0",
		"trace_exec":         "trace_exec@sha256:e774e2be4a33b77b0a5f3e56b2032ef8df2f3c5419a7f6ac2aa22b4c062bf766",
		"trace_tcp":          "trace_tcp@sha256:6013fa661ca78925c621c1d63a5fe31bfb2519aed977ebe641856f43fc960234",
		"trace_bind":         "trace_bind@sha256:a22a835b94ef66f86bee23d931890c746407d6787f47556ad4965e53eeb5ce86",
		"trace_capabilities": "trace_capabilities@sha256:cf52436b7348fc80f6388c1de6a6c90973d9c285497be5b177b7cd6d2cf107b4",
		"advise_seccomp":     "advise_seccomp@sha256:79e050a8aa4be204da503efc722db89edc2be62245336eac5d07b7d045fc66d0",
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
