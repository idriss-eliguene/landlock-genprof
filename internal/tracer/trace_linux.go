// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

//go:build linux

package tracer

import (
	"context"
	"fmt"
	"time"

	"github.com/inspektor-gadget/inspektor-gadget/pkg/datasource"
	gadgetcontext "github.com/inspektor-gadget/inspektor-gadget/pkg/gadget-context"
	"github.com/inspektor-gadget/inspektor-gadget/pkg/operators"
	"github.com/inspektor-gadget/inspektor-gadget/pkg/operators/simple"
	grpcruntime "github.com/inspektor-gadget/inspektor-gadget/pkg/runtime/grpc"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
)

// traceOpenImage is the OCI image of the trace_open gadget (see
// https://github.com/inspektor-gadget/inspektor-gadget/blob/main/docs/gadgets/trace_open.mdx).
// Deliberately `:latest`, not pinned to the ig/kubectl-gadget CLI version
// (v0.54.1): gadget images have their own release cycle, decoupled from
// the CLI tools — trace_open:v0.54.1 doesn't exist (verified against
// ghcr.io directly; its latest real version tag is v0.27.0). Matches
// HOW_TO_START.md §5's note on the same gotcha.
const traceOpenImage = "trace_open:latest"

// openAccessModeMask isolates the access-mode bits (O_RDONLY/O_WRONLY/O_RDWR)
// from a raw openat(2) flags value — the low 2 bits per POSIX (O_ACCMODE).
const openAccessModeMask = 0x3

// Trace starts an Inspektor Gadget trace_open capture against the target
// pod/container for Duration, and returns the observed file-open events.
//
// This runs the equivalent of:
//
//	kubectl gadget run trace_open:latest -n <namespace> -c <container>
//
// programmatically via the gRPC runtime, against the Inspektor Gadget
// DaemonSet already deployed on the cluster (see hack/init-vm.sh) —
// nothing here deploys or manages that DaemonSet itself.
func Trace(opts Options) ([]Event, error) {
	config, err := k8s.RestConfig()
	if err != nil {
		return nil, fmt.Errorf("kubernetes config: %w", err)
	}

	var events []Event

	const collectorPriority = 50000
	collector := simple.New("landlock-genprof-collector",
		simple.OnInit(func(gadgetCtx operators.GadgetContext) error {
			for _, ds := range gadgetCtx.GetDataSources() {
				fnameField := ds.GetField("fname")
				flagsField := ds.GetField("flags_raw")
				errorField := ds.GetField("error_raw")
				timestampField := ds.GetField("timestamp_raw")

				ds.Subscribe(func(source datasource.DataSource, data datasource.Data) error {
					// Skip failed opens (ENOENT, EACCES, ...): a path that
					// was never successfully accessed shouldn't become a
					// Landlock allow-rule.
					if errno, err := errorField.Uint32(data); err != nil || errno != 0 {
						return nil
					}

					fname, err := fnameField.String(data)
					if err != nil || fname == "" {
						return nil
					}

					flags, err := flagsField.Uint32(data)
					if err != nil {
						return nil
					}

					var timestamp time.Time
					if ts, err := timestampField.Uint64(data); err == nil {
						timestamp = time.Unix(0, int64(ts))
					}

					events = append(events, Event{
						Timestamp: timestamp,
						Syscall:   "openat",
						Path:      fname,
						Mode:      modeFromOpenFlags(flags),
					})
					return nil
				}, collectorPriority)
			}
			return nil
		}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), opts.Duration)
	defer cancel()

	gadgetCtx := gadgetcontext.New(
		ctx,
		traceOpenImage,
		gadgetcontext.WithDataOperators(collector),
	)

	runtime := grpcruntime.New(grpcruntime.WithConnectUsingK8SProxy)
	runtime.SetRestConfig(config)

	if err := runtime.Init(runtime.GlobalParamDescs().ToParams()); err != nil {
		return nil, fmt.Errorf("gadget runtime init: %w", err)
	}
	defer runtime.Close()

	filterParams := map[string]string{
		"operator.KubeManager.namespace":     opts.Namespace,
		"operator.KubeManager.podname":       opts.PodName,
		"operator.KubeManager.containername": opts.Container,
	}

	if err := runtime.RunGadget(gadgetCtx, nil, filterParams); err != nil {
		return nil, fmt.Errorf("running trace_open gadget: %w", err)
	}

	return events, nil
}

// modeFromOpenFlags maps a raw openat(2) flags value to our simplified
// read/write/read_write vocabulary, using the standard POSIX O_ACCMODE
// convention (O_RDONLY=0, O_WRONLY=1, O_RDWR=2) — independent of any
// Inspektor Gadget version, since it comes from the kernel's open(2) ABI.
func modeFromOpenFlags(flags uint32) string {
	switch flags & openAccessModeMask {
	case 1:
		return "write"
	case 2:
		return "read_write"
	default:
		return "read"
	}
}
