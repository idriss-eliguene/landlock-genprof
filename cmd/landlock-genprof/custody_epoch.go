package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/idriss-eliguene/landlock-genprof/internal/attempt"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
)

var newDynamicClientForCustodyEpoch = newDynamicClient

func newCustodyEpochCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "custody-epoch", Short: "Manages the ApplyAttempt custody qualification epoch"}
	cmd.AddCommand(&cobra.Command{
		Use: "activate", Short: "Proves CRD hardening and publishes a fresh custody epoch",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCustodyEpochActivate(cmd.Context(), cmd.OutOrStdout())
		},
	})
	return cmd
}

func runCustodyEpochActivate(ctx context.Context, stdout io.Writer) error {
	client, err := newDynamicClientForCustodyEpoch()
	if err != nil {
		return err
	}
	crd, err := client.Resource(attempt.CRDGVR).Get(ctx, "applyattempts.landlockgenprof.io", metav1.GetOptions{})
	if err != nil {
		return err
	}
	if !crdEstablished(crd) {
		return fmt.Errorf("ApplyAttempt CRD is not Established")
	}
	_, hardened, err := attempt.CustodyEpochAndHardening(ctx, client)
	if err != nil {
		return err
	}
	if !hardened {
		return fmt.Errorf("ApplyAttempt CRD does not expose the required hardening validation")
	}
	probeName, probe, err := attempt.Create(ctx, client, "default", attempt.Spec{ProposalNamespace: "default", ProposalName: "custody-epoch-probe", ProposalUID: "probe", ApprovedCandidateDigest: "sha256:" + fmt.Sprintf("%064d", 1), Target: k8s.GovernedTarget{Namespace: "default", Workload: k8s.WorkloadRef{Kind: "Pod", Name: "custody-epoch-probe"}, Container: "probe"}, StartedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return fmt.Errorf("hardening probe create: %w", err)
	}
	defer func() {
		_ = client.Resource(attempt.GVR).Namespace("default").Delete(context.Background(), probeName, metav1.DeleteOptions{})
	}()
	if err := attempt.SaveStatus(ctx, client, "default", probeName, probe, attempt.Status{State: attempt.StateInProgress}); err != nil {
		return err
	}
	if err := attempt.SaveStatus(ctx, client, "default", probeName, probe, attempt.Status{State: attempt.StateApplied}); err != nil {
		return err
	}
	if err := attempt.SaveStatus(ctx, client, "default", probeName, probe, attempt.Status{State: attempt.StateFailed}); err == nil || !apierrors.IsInvalid(err) {
		return fmt.Errorf("hardening probe did not reject terminal status rewrite: %v", err)
	}
	epoch, err := attempt.NewCustodyEpoch()
	if err != nil {
		return err
	}
	if _, err := attempt.PatchCustodyEpoch(ctx, client, epoch); err != nil {
		return fmt.Errorf("publishing custody epoch: %w", err)
	}
	current, err := client.Resource(attempt.CRDGVR).Get(ctx, "applyattempts.landlockgenprof.io", metav1.GetOptions{})
	if err != nil {
		return err
	}
	if current.GetAnnotations()[attempt.CustodyEpochAnnotation] != epoch {
		return fmt.Errorf("custody epoch publication could not be confirmed")
	}
	fmt.Fprintf(stdout, "custody epoch activated: %s\n", epoch)
	return nil
}

func crdEstablished(crd *unstructured.Unstructured) bool {
	conditions, _, _ := unstructured.NestedSlice(crd.Object, "status", "conditions")
	for _, raw := range conditions {
		c, ok := raw.(map[string]interface{})
		if ok && c["type"] == "Established" && c["status"] == "True" {
			return true
		}
	}
	return false
}
