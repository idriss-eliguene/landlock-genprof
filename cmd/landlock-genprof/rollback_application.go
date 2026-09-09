package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	rollbackapp "github.com/idriss-eliguene/landlock-genprof/internal/rollback"
)

func runRollback(ctx context.Context, stdout io.Writer, stdin io.Reader, opts rollbackOptions, name string) error {
	return rollbackapp.Run(ctx, stdout, stdin, rollbackapp.Options{
		Namespace: opts.namespace,
		Yes:       opts.yes,
	}, name, rollbackapp.Dependencies{
		NewDynamicClient:          newDynamicClientForRollback,
		CreateRollbackAttempt:     createRollbackAttempt,
		SaveRollbackAttemptStatus: saveRollbackAttemptStatus,
		ExecuteInverse:            executeInverseForRollback,
		InverseFailureResult:      inverseFailureResult,
		Confirm: func(out io.Writer, in io.Reader) bool {
			fmt.Fprint(out, "Execute this rollback? [y/N] ")
			line, _ := bufio.NewReader(in).ReadString('\n')
			answer := strings.ToLower(strings.TrimSpace(line))
			return answer == "y" || answer == "yes"
		},
	})
}
