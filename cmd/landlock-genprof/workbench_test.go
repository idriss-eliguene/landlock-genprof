// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	discoveryfake "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
)

// workbenchReadFixture builds a bounded k8s.WorkbenchReadCapability backed by
// fake clients, and the write-capable dynamic.Interface used only to seed
// fixtures through the real proposal.Save/SetApprovalState — exactly the
// split G3 requires: tests may still write fixtures with a broad client, but
// Shared read-model tests must only ever see the bounded capability.
func workbenchReadFixture(t *testing.T, namespace string) (dynamic.Interface, k8s.WorkbenchReadCapability) {
	t.Helper()
	core := kubefake.NewSimpleClientset()
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "securityprofileproposals"}:        "SecurityProfileProposalList",
		{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "observations"}:                    "ObservationList",
		{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "traininghistories"}:               "TrainingHistoryList",
		{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "applyattempts"}:                   "ApplyAttemptList",
		{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "rollbackattempts"}:                "RollbackAttemptList",
		{Group: "landlockgenprof.io", Version: "v1alpha1", Resource: "observationcontributionreceipts"}: "ObservationContributionReceiptList",
	})
	disc := core.Discovery().(*discoveryfake.FakeDiscovery)
	disc.Resources = []*metav1.APIResourceList{
		{GroupVersion: "landlockgenprof.io/v1alpha1", APIResources: []metav1.APIResource{
			{Name: "securityprofileproposals"}, {Name: "observations"}, {Name: "traininghistories"},
			{Name: "applyattempts"}, {Name: "rollbackattempts"},
			{Name: "observationcontributionreceipts"},
		}},
	}
	reads, err := k8s.NewReadSessionForClients(core, dyn, core.Discovery(), namespace)
	if err != nil {
		t.Fatalf("k8s.NewReadSessionForClients() error = %v", err)
	}
	return dyn, reads
}

func TestWorkbenchListenAddress_IsLoopbackOnly(t *testing.T) {
	t.Setenv(workbenchDeploymentModeEnv, "local")
	if got := workbenchListenAddress(8080); got != "127.0.0.1:8080" {
		t.Fatalf("workbenchListenAddress() = %q, want loopback address", got)
	}
}

func TestWorkbenchListenAddress_ProductionIsPodReachable(t *testing.T) {
	t.Setenv(workbenchDeploymentModeEnv, "production")
	if got := workbenchListenAddress(8080); got != "0.0.0.0:8080" {
		t.Fatalf("workbenchListenAddress() = %q, want pod-reachable address", got)
	}
}

func TestWorkbenchReadHeaderTimeout_IsNonZero(t *testing.T) {
	if workbenchReadHeaderTimeout <= 0 || workbenchReadHeaderTimeout != 5*time.Second {
		t.Fatalf("workbenchReadHeaderTimeout = %s, want 5s", workbenchReadHeaderTimeout)
	}
}

// TestWorkbenchServerBounds_AreAllNonZero pins every configured resource
// bound. A zero value here would mean an unbounded Slowloris/header/idle/
// concurrency/response surface; #185 requires all of these to be explicit.
func TestWorkbenchServerBounds_AreAllNonZero(t *testing.T) {
	if workbenchReadTimeout <= 0 {
		t.Error("workbenchReadTimeout is not positive")
	}
	if workbenchWriteTimeout <= 0 {
		t.Error("workbenchWriteTimeout is not positive")
	}
	if workbenchWriteTimeout <= workbenchClusterReadDeadline {
		t.Errorf("workbenchWriteTimeout (%s) must exceed the cluster-read deadline (%s), or a legitimate slow read gets killed by the transport instead of returning its own timeout", workbenchWriteTimeout, workbenchClusterReadDeadline)
	}
	if workbenchIdleTimeout <= 0 {
		t.Error("workbenchIdleTimeout is not positive")
	}
	if workbenchMaxHeaderBytes <= 0 {
		t.Error("workbenchMaxHeaderBytes is not positive")
	}
	if workbenchClusterReadDeadline <= 0 {
		t.Error("workbenchClusterReadDeadline is not positive")
	}
	if workbenchMaxConcurrentReads <= 0 {
		t.Error("workbenchMaxConcurrentReads is not positive")
	}
	if workbenchMaxResponseBytes <= 0 {
		t.Error("workbenchMaxResponseBytes is not positive")
	}
	if workbenchMaxRequestBodyBytes <= 0 {
		t.Error("workbenchMaxRequestBodyBytes is not positive")
	}
}
