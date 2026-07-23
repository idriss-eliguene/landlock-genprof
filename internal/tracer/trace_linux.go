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
	"math"
	"path"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"
	"k8s.io/client-go/rest"

	"github.com/inspektor-gadget/inspektor-gadget/pkg/datasource"
	gadgetcontext "github.com/inspektor-gadget/inspektor-gadget/pkg/gadget-context"
	"github.com/inspektor-gadget/inspektor-gadget/pkg/operators"
	"github.com/inspektor-gadget/inspektor-gadget/pkg/operators/simple"
	grpcruntime "github.com/inspektor-gadget/inspektor-gadget/pkg/runtime/grpc"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
)

// traceOpenImage and traceExecImage are the OCI images of the trace_open
// and trace_exec gadgets (see
// https://github.com/inspektor-gadget/inspektor-gadget/blob/main/docs/gadgets/trace_open.mdx
// and .../trace_exec.mdx). Deliberately `:latest`, not pinned to the
// ig/kubectl-gadget CLI version (v0.54.1): gadget images have their own
// release cycle, decoupled from the CLI tools — trace_open:v0.54.1
// doesn't exist (verified against ghcr.io directly; its latest real
// version tag is v0.27.0). Matches HOW_TO_START.md §5's note on the same
// gotcha.
const (
	traceOpenImage       = "trace_open:latest"
	traceExecImage       = "trace_exec:latest"
	traceTCPConnectImage = "trace_tcpconnect:latest"
	traceBindImage       = "trace_bind:latest"
)

// openAccessModeMask isolates the access-mode bits (O_RDONLY/O_WRONLY/O_RDWR)
// from a raw openat(2) flags value — the low 2 bits per POSIX (O_ACCMODE).
const openAccessModeMask = 0x3

// requireField looks up a field by name and errors out immediately if it
// doesn't exist, instead of returning a nil FieldAccessor that panics the
// first time something calls .Uint16()/.String()/etc. on it.
//
// This is exactly the kind of external boundary worth validating: a
// gadget's field set is defined by its own upstream schema, not by this
// codebase, and a wrong guess here is a real, previously-hit failure
// mode — trace_bind's port field was first guessed as "port", which
// doesn't exist and crashed with a nil pointer dereference the first
// time it was exercised against a live cluster, instead of surfacing a
// clear error. Every one of the four run*Tracer functions below goes
// through this, including trace_open/trace_exec's already-confirmed
// field names: an upstream gadget schema change would otherwise
// reintroduce the exact same crash silently.
func requireField(ds datasource.DataSource, name string) (datasource.FieldAccessor, error) {
	field := ds.GetField(name)
	if field == nil {
		return nil, fmt.Errorf("data source %q has no field %q", ds.Name(), name)
	}
	return field, nil
}

// commMaxLen is TASK_COMM_LEN (16) minus the null terminator: the kernel
// always truncates a process's comm to this length, so every gadget's
// "comm" field is too — comparing against an untruncated basename would
// silently never match for any binary with a longer name.
const commMaxLen = 15

// commFromBinaryPath derives the comm a successful exec of binary would
// report, for scoping capture to the traced process — see the comm
// filtering in each run*Tracer function below and docs/e2e-demo.md
// Finding 1 (training-run contamination: without this, a `kubectl exec
// ... -- ls` sharing the pod's namespaces during the training window was
// captured and attributed to the traced binary indistinguishably from
// its own activity).
func commFromBinaryPath(binary string) string {
	comm := path.Base(binary)
	if len(comm) > commMaxLen {
		comm = comm[:commMaxLen]
	}
	return comm
}

// Trace starts Inspektor Gadget captures against the target pod/container
// for Duration, and returns the observed events.
//
// It runs four gadgets concurrently:
//
//	kubectl gadget run trace_open:latest -n <namespace> -c <container>
//	kubectl gadget run trace_exec:latest --paths -n <namespace> -c <container>
//	kubectl gadget run trace_tcpconnect:latest -n <namespace> -c <container>
//	kubectl gadget run trace_bind:latest -n <namespace> -c <container>
//
// trace_open alone cannot tell us that a path was *executed*: openat(2)
// has no exec bit in its flags (O_ACCMODE only distinguishes
// read/write/read_write — unlike FreeBSD, Linux has no O_EXEC). Landlock's
// own execute right therefore needs a separate signal, which is exactly
// what trace_exec (hooking execve/execveat, not openat) provides. This gap
// was only discovered by testing end to end against a live cluster: hand
// -crafted unit test events had always included a Mode of "exec" directly,
// which no code path in this file could ever actually produce — see
// docs/policy-synthesis.md.
//
// trace_tcpconnect/trace_bind cover Landlock's LANDLOCK_ACCESS_NET_CONNECT_TCP/
// LANDLOCK_ACCESS_NET_BIND_TCP rights (kernel >= 6.4, see README's gadget
// table). These were deferred for a while because the only exporter
// (PodLock) has no field to represent network rights at all — that's still
// true, but the internal/exporter/networkpolicy exporter now gives this
// data a real destination, so the tracer side is no longer held back for
// that reason (see docs/roadmap.md).
//
// All four run programmatically via the gRPC runtime, against the
// Inspektor Gadget DaemonSet already deployed on the cluster (see
// hack/init-vm.sh) — nothing here deploys or manages that DaemonSet
// itself.
//
// Every event is additionally scoped to processes whose comm matches
// opts.Binary's basename (see commFromBinaryPath and each run*Tracer's
// comm check): the Inspektor Gadget filterParams below only scope by
// pod/namespace/container, which a `kubectl exec` session shares with the
// traced binary — see docs/e2e-demo.md Finding 1 for the real
// contamination this caused before this check existed.
func Trace(opts Options) ([]Event, error) {
	config, err := k8s.RestConfig()
	if err != nil {
		return nil, fmt.Errorf("kubernetes config: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Duration)
	defer cancel()

	filterParams := map[string]string{
		"operator.KubeManager.namespace":     opts.Namespace,
		"operator.KubeManager.podname":       opts.PodName,
		"operator.KubeManager.containername": opts.Container,
	}
	expectedComm := commFromBinaryPath(opts.Binary)

	var (
		mu     sync.Mutex
		events []Event
	)
	emit := func(ev Event) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	}

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return runOpenTracer(gctx, config, filterParams, expectedComm, emit)
	})
	g.Go(func() error {
		return runExecTracer(gctx, config, filterParams, expectedComm, emit)
	})
	g.Go(func() error {
		return runConnectTracer(gctx, config, filterParams, expectedComm, emit)
	})
	g.Go(func() error {
		return runBindTracer(gctx, config, filterParams, expectedComm, emit)
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return events, nil
}

// runOpenTracer runs the trace_open gadget and emits one Event per
// successful openat(2), mapping its flags to our read/write/read_write
// vocabulary (see modeFromOpenFlags).
//
// expectedComm scopes capture to the traced binary: field name
// "proc.comm", confirmed against a live cluster's `kubectl gadget run
// trace_open:latest -o json` output — nested under "proc", like the
// network gadgets' comm field, *not* flat alongside this gadget's other
// fields (fname/flags_raw/error_raw/timestamp_raw) as first guessed
// (that guess failed cleanly via requireField below: "data source open
// has no field comm", no crash). See commFromBinaryPath's comment and
// docs/e2e-demo.md Finding 1.
func runOpenTracer(ctx context.Context, config *rest.Config, filterParams map[string]string, expectedComm string, emit func(Event)) error {
	const collectorPriority = 50000
	collector := simple.New("landlock-genprof-open-collector",
		simple.OnInit(func(gadgetCtx operators.GadgetContext) error {
			for _, ds := range gadgetCtx.GetDataSources() {
				fnameField, err := requireField(ds, "fname")
				if err != nil {
					return err
				}
				flagsField, err := requireField(ds, "flags_raw")
				if err != nil {
					return err
				}
				commField, err := requireField(ds, "proc.comm")
				if err != nil {
					return err
				}
				errorField, err := requireField(ds, "error_raw")
				if err != nil {
					return err
				}
				timestampField, err := requireField(ds, "timestamp_raw")
				if err != nil {
					return err
				}

				err = ds.Subscribe(func(source datasource.DataSource, data datasource.Data) error {
					// Skip failed opens (ENOENT, EACCES, ...): a path that
					// was never successfully accessed shouldn't become a
					// Landlock allow-rule.
					if errno, err := errorField.Uint32(data); err != nil || errno != 0 {
						return nil
					}

					// Skip events from any process other than the traced
					// binary — see expectedComm's doc comment above.
					if comm, err := commField.String(data); err != nil || comm != expectedComm {
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

					emit(Event{
						Timestamp: timestampFromRaw(timestampField, data),
						Syscall:   "openat",
						Path:      fname,
						Mode:      modeFromOpenFlags(flags),
						IsDir:     flags&unix.O_DIRECTORY != 0,
					})
					return nil
				}, collectorPriority)
				if err != nil {
					return fmt.Errorf("subscribing to data source %q: %w", ds.Name(), err)
				}
			}
			return nil
		}),
	)

	gadgetCtx := gadgetcontext.New(
		ctx,
		traceOpenImage,
		gadgetcontext.WithDataOperators(collector),
	)

	runtime := grpcruntime.New(grpcruntime.WithConnectUsingK8SProxy)
	runtime.SetRestConfig(config)

	if err := runtime.Init(runtime.GlobalParamDescs().ToParams()); err != nil {
		return fmt.Errorf("gadget runtime init: %w", err)
	}
	defer runtime.Close()

	if err := runtime.RunGadget(gadgetCtx, nil, filterParams); err != nil {
		return fmt.Errorf("running trace_open gadget: %w", err)
	}
	return nil
}

// runExecTracer runs the trace_exec gadget and emits one Event per
// successful execve(2)/execveat(2), tagged Mode "exec".
//
// The gadget's exepath/file fields (the executed binary, and — in the
// shebang-script case — the script file itself, which can differ from
// exepath) are only populated when its "paths" eBPF param is enabled
// (default false, see gadgets/trace_exec/gadget.yaml upstream): hence
// "operator.oci.ebpf.paths" = "true" below (confirmed against the real
// param identifier via runtime.GetGadgetInfo() — see the comment further
// down), on top of the usual KubeManager pod/namespace/container filter.
//
// expectedComm scopes capture to the traced binary — see
// runOpenTracer's comm doc comment ("proc.comm", confirmed the same way).
// Deliberate limitation: a
// legitimate child process the traced binary execs under a *different*
// comm (e.g. a CGI script) is filtered out too — a new false negative
// traded for closing a demonstrated false positive; not a concern for
// the nginx demo config (no exec directive), but worth knowing for a
// future target that does spawn differently-named children.
func runExecTracer(ctx context.Context, config *rest.Config, filterParams map[string]string, expectedComm string, emit func(Event)) error {
	const collectorPriority = 50000
	collector := simple.New("landlock-genprof-exec-collector",
		simple.OnInit(func(gadgetCtx operators.GadgetContext) error {
			for _, ds := range gadgetCtx.GetDataSources() {
				exepathField, err := requireField(ds, "exepath")
				if err != nil {
					return err
				}
				fileField, err := requireField(ds, "file")
				if err != nil {
					return err
				}
				commField, err := requireField(ds, "proc.comm")
				if err != nil {
					return err
				}
				errorField, err := requireField(ds, "error_raw")
				if err != nil {
					return err
				}
				timestampField, err := requireField(ds, "timestamp_raw")
				if err != nil {
					return err
				}

				err = ds.Subscribe(func(source datasource.DataSource, data datasource.Data) error {
					// Skip failed execs: a binary that was never
					// successfully executed shouldn't become an exec rule.
					if errno, err := errorField.Uint32(data); err != nil || errno != 0 {
						return nil
					}

					// Skip events from any process other than the traced
					// binary — see expectedComm's doc comment above.
					if comm, err := commField.String(data); err != nil || comm != expectedComm {
						return nil
					}

					ts := timestampFromRaw(timestampField, data)

					exepath, err := exepathField.String(data)
					if err == nil && exepath != "" {
						emit(Event{
							Timestamp: ts,
							Syscall:   "execve",
							Path:      exepath,
							Mode:      "exec",
						})
					}

					// In the shebang-script case, `file` (the script) can
					// differ from `exepath` (the interpreter, e.g.
					// /bin/sh) — the script itself was also part of the
					// exec chain and needs its own rule.
					file, err := fileField.String(data)
					if err == nil && file != "" && file != exepath {
						emit(Event{
							Timestamp: ts,
							Syscall:   "execve",
							Path:      file,
							Mode:      "exec",
						})
					}

					return nil
				}, collectorPriority)
				if err != nil {
					return fmt.Errorf("subscribing to data source %q: %w", ds.Name(), err)
				}
			}
			return nil
		}),
	)

	gadgetCtx := gadgetcontext.New(
		ctx,
		traceExecImage,
		gadgetcontext.WithDataOperators(collector),
	)

	runtime := grpcruntime.New(grpcruntime.WithConnectUsingK8SProxy)
	runtime.SetRestConfig(config)

	if err := runtime.Init(runtime.GlobalParamDescs().ToParams()); err != nil {
		return fmt.Errorf("gadget runtime init: %w", err)
	}
	defer runtime.Close()

	execParams := make(map[string]string, len(filterParams)+1)
	for k, v := range filterParams {
		execParams[k] = v
	}
	// Confirmed via runtime.GetGadgetInfo() against the live cluster: the
	// "ebpf" operator's per-image params (declared via GADGET_PARAM in the
	// gadget's C source) are nested under "operator.oci.ebpf.", not
	// "operator.ebpf." directly — the "oci" operator owns a per-image "ebpf"
	// sub-instance, so the prefix compounds. Guessing "operator.ebpf.paths"
	// first silently did nothing (unknown params aren't rejected, just
	// ignored), which is why exepath/file came back empty despite the
	// gadget capturing the exec events fine (verified via the raw CLI).
	execParams["operator.oci.ebpf.paths"] = "true"

	if err := runtime.RunGadget(gadgetCtx, nil, execParams); err != nil {
		return fmt.Errorf("running trace_exec gadget: %w", err)
	}
	return nil
}

// runConnectTracer runs the trace_tcpconnect gadget and emits one Event
// per successful connect(2), tagged Mode "egress" with the destination
// port.
//
// Field name confirmed end to end against a live cluster (both via
// `kubectl gadget run trace_tcpconnect:latest -o json`, which showed the
// destination port as a sub-field of a nested "dst" struct —
// {"dst":{"addr":"1.1.1.1","port":80,...}} — and via a real
// landlock-genprof trace run producing a correct egress port in the
// generated NetworkPolicy): "dst.port", not a flat "dport" as originally
// guessed. Dot-path access for nested fields matches the vendored SDK's
// own generate_networkpolicy operator (see
// pkg/operators/generate_networkpolicy/generate_networkpolicy_op.go:130,
// `ds.GetField("endpoint.port")`).
//
// expectedComm scopes capture to the traced binary — field name
// "proc.comm", already confirmed this session from the real `kubectl
// gadget run trace_tcpconnect:latest -o json` capture
// ({"proc":{"comm":"wget",...}}). See docs/e2e-demo.md Finding 1
// (originally about trace_open/trace_exec, but docs/threat-model.md
// notes the same contamination risk applies to the network tracers).
func runConnectTracer(ctx context.Context, config *rest.Config, filterParams map[string]string, expectedComm string, emit func(Event)) error {
	const collectorPriority = 50000
	collector := simple.New("landlock-genprof-connect-collector",
		simple.OnInit(func(gadgetCtx operators.GadgetContext) error {
			for _, ds := range gadgetCtx.GetDataSources() {
				dportField, err := requireField(ds, "dst.port")
				if err != nil {
					return err
				}
				commField, err := requireField(ds, "proc.comm")
				if err != nil {
					return err
				}
				errorField, err := requireField(ds, "error_raw")
				if err != nil {
					return err
				}
				timestampField, err := requireField(ds, "timestamp_raw")
				if err != nil {
					return err
				}

				err = ds.Subscribe(func(source datasource.DataSource, data datasource.Data) error {
					// Skip failed connects: a port that was never
					// successfully reached shouldn't become an egress rule.
					if errno, err := errorField.Uint32(data); err != nil || errno != 0 {
						return nil
					}

					// Skip events from any process other than the traced
					// binary — see expectedComm's doc comment above.
					if comm, err := commField.String(data); err != nil || comm != expectedComm {
						return nil
					}

					dport, err := dportField.Uint16(data)
					if err != nil || dport == 0 {
						return nil
					}

					emit(Event{
						Timestamp: timestampFromRaw(timestampField, data),
						Syscall:   "connect",
						Port:      int(dport),
						Mode:      "egress",
					})
					return nil
				}, collectorPriority)
				if err != nil {
					return fmt.Errorf("subscribing to data source %q: %w", ds.Name(), err)
				}
			}
			return nil
		}),
	)

	gadgetCtx := gadgetcontext.New(
		ctx,
		traceTCPConnectImage,
		gadgetcontext.WithDataOperators(collector),
	)

	runtime := grpcruntime.New(grpcruntime.WithConnectUsingK8SProxy)
	runtime.SetRestConfig(config)

	if err := runtime.Init(runtime.GlobalParamDescs().ToParams()); err != nil {
		return fmt.Errorf("gadget runtime init: %w", err)
	}
	defer runtime.Close()

	if err := runtime.RunGadget(gadgetCtx, nil, filterParams); err != nil {
		return fmt.Errorf("running trace_tcpconnect gadget: %w", err)
	}
	return nil
}

// runBindTracer runs the trace_bind gadget and emits one Event per
// successful bind(2), tagged Mode "ingress" with the bound port.
//
// Field name confirmed end to end against a live cluster: a flat "port"
// was the first guess and confirmed wrong (requireField below turned
// that into a clean error instead of the nil-pointer panic a raw
// ds.GetField("port") caused). "addr.port" — by analogy with
// trace_tcpconnect's "dst.port" nesting (see runConnectTracer) — is
// confirmed correct: a real landlock-genprof trace run produced the
// expected ingress port in the generated NetworkPolicy. See
// docs/policy-synthesis.md for a real false positive this surfaced
// (ephemeral client-side ports look identical to a real bind(2) at the
// syscall level trace_bind hooks — filtered in internal/policy.Synthesize).
//
// expectedComm scopes capture to the traced binary — field name
// "proc.comm", by analogy with runConnectTracer's confirmed nesting for
// the same gadget family; not directly confirmed by observation for
// trace_bind specifically. See docs/e2e-demo.md Finding 1 /
// docs/threat-model.md's network contamination note.
func runBindTracer(ctx context.Context, config *rest.Config, filterParams map[string]string, expectedComm string, emit func(Event)) error {
	const collectorPriority = 50000
	collector := simple.New("landlock-genprof-bind-collector",
		simple.OnInit(func(gadgetCtx operators.GadgetContext) error {
			for _, ds := range gadgetCtx.GetDataSources() {
				portField, err := requireField(ds, "addr.port")
				if err != nil {
					return err
				}
				commField, err := requireField(ds, "proc.comm")
				if err != nil {
					return err
				}
				errorField, err := requireField(ds, "error_raw")
				if err != nil {
					return err
				}
				timestampField, err := requireField(ds, "timestamp_raw")
				if err != nil {
					return err
				}

				err = ds.Subscribe(func(source datasource.DataSource, data datasource.Data) error {
					// Skip failed binds: a port that was never
					// successfully bound shouldn't become an ingress rule.
					if errno, err := errorField.Uint32(data); err != nil || errno != 0 {
						return nil
					}

					// Skip events from any process other than the traced
					// binary — see expectedComm's doc comment above.
					if comm, err := commField.String(data); err != nil || comm != expectedComm {
						return nil
					}

					port, err := portField.Uint16(data)
					if err != nil || port == 0 {
						return nil
					}

					emit(Event{
						Timestamp: timestampFromRaw(timestampField, data),
						Syscall:   "bind",
						Port:      int(port),
						Mode:      "ingress",
					})
					return nil
				}, collectorPriority)
				if err != nil {
					return fmt.Errorf("subscribing to data source %q: %w", ds.Name(), err)
				}
			}
			return nil
		}),
	)

	gadgetCtx := gadgetcontext.New(
		ctx,
		traceBindImage,
		gadgetcontext.WithDataOperators(collector),
	)

	runtime := grpcruntime.New(grpcruntime.WithConnectUsingK8SProxy)
	runtime.SetRestConfig(config)

	if err := runtime.Init(runtime.GlobalParamDescs().ToParams()); err != nil {
		return fmt.Errorf("gadget runtime init: %w", err)
	}
	defer runtime.Close()

	if err := runtime.RunGadget(gadgetCtx, nil, filterParams); err != nil {
		return fmt.Errorf("running trace_bind gadget: %w", err)
	}
	return nil
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

// timestampFromRaw converts a gadget's timestamp_raw (nanoseconds since
// boot, __u64) to a time.Time. Explicitly bounds-checked before the
// uint64->int64 conversion time.Unix needs (gosec G115): in practice a
// boot-relative nanosecond count can never realistically approach
// math.MaxInt64 (that's ~292 years of uptime), but nothing stops the
// field accessor from handing back a garbage value on a malformed event,
// and silently wrapping to a negative timestamp would be worse than just
// leaving it zero.
func timestampFromRaw(field datasource.FieldAccessor, data datasource.Data) time.Time {
	ts, err := field.Uint64(data)
	if err != nil || ts > math.MaxInt64 {
		return time.Time{}
	}
	return time.Unix(0, int64(ts))
}
