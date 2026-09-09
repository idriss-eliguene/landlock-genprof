package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"k8s.io/client-go/dynamic"

	"github.com/idriss-eliguene/landlock-genprof/internal/applyproposal"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/proposal"
)

func runApplyProposalInternal(ctx context.Context, stdout io.Writer, stdin io.Reader, opts applyProposalOptions, proposalName string, certification bool) error {
	deps := applyproposal.Dependencies{
		NewDynamicClient:      newDynamicClientForApplyProposal,
		SaveAttemptStatus:     saveAttemptStatus,
		CreateAttempt:         createAttempt,
		ReadApplyResource:     readApplyResource,
		AfterPlanBuilt:        afterApplyProposalPlanBuilt,
		AfterEnforcementReady: afterEnforcementReady,
		PrintSummary: func(out io.Writer, namespace, name string, spec *proposal.Spec, status *proposal.Status) {
			printProposalSummary(out, namespace, name, spec, status, proposalArtifacts(spec))
		},
		Confirm: func(out io.Writer, in io.Reader) bool {
			fmt.Fprint(out, "Apply these planned artifacts to the cluster? [y/N] ")
			line, _ := bufio.NewReader(in).ReadString('\n')
			answer := strings.ToLower(strings.TrimSpace(line))
			return answer == "y" || answer == "yes"
		},
		ApplyManifestObserved: func(c context.Context, client dynamic.Interface, namespace, content string, guard k8s.ApplyGuard) (k8s.MutationObservation, error) {
			currentApplyGuard = &guard
			defer func() { currentApplyGuard = nil }()
			return applyManifestObserved(c, client, namespace, content)
		},
	}
	return applyproposal.Run(ctx, stdout, stdin, applyproposal.Options{
		Namespace:        opts.namespace,
		Yes:              opts.yes,
		Skip:             opts.skip,
		Restart:          opts.restart,
		ReadinessTimeout: opts.readinessTimeout,
	}, proposalName, certification, deps)
}
