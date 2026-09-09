// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package observationapp contains reusable application orchestration for
// creating Observations. It deliberately does not choose how the runtime
// Runner is executed: callers may run it synchronously or asynchronously.
package observationapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	obskube "github.com/idriss-eliguene/landlock-genprof/internal/observation/kubernetes"
	"github.com/idriss-eliguene/landlock-genprof/internal/observation/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type Clients struct {
	Core    kubernetes.Interface
	Dynamic dynamic.Interface
}

type PrepareRequest struct {
	Namespace string
	Pod       string
	Container string
	Sources   []string
	Duration  time.Duration
	Requester string
}

type Prepared struct {
	ID      domain.ObservationID
	Spec    domain.ObservationSpec
	Target  k8s.ObservationTarget
	Cluster domain.ClusterIdentity
	Sources []runtime.FilesystemSource
	Store   *obskube.Store
}

func Prepare(ctx context.Context, clients Clients, request PrepareRequest) (Prepared, error) {
	if clients.Core == nil || clients.Dynamic == nil {
		return Prepared{}, fmt.Errorf("observation preparation requires Kubernetes clients")
	}
	if strings.TrimSpace(request.Namespace) == "" || strings.TrimSpace(request.Pod) == "" || strings.TrimSpace(request.Container) == "" {
		return Prepared{}, fmt.Errorf("invalid request: namespace, pod, and container are required")
	}
	if request.Duration <= 0 || request.Duration > 24*time.Hour {
		return Prepared{}, fmt.Errorf("invalid request: duration must be between 1ns and 24h")
	}
	cluster, err := k8s.ResolveClusterIdentity(ctx, clients.Core)
	if err != nil {
		return Prepared{}, fmt.Errorf("cluster identity unresolved: %w", err)
	}
	pod, err := clients.Core.CoreV1().Pods(request.Namespace).Get(ctx, request.Pod, metav1.GetOptions{})
	if err != nil {
		return Prepared{}, fmt.Errorf("invalid target: %w", err)
	}
	target, err := k8s.ResolveObservationTarget(ctx, clients.Core, cluster, pod, request.Container)
	if err != nil {
		return Prepared{}, fmt.Errorf("invalid target: %w", err)
	}
	if len(request.Sources) == 0 {
		request.Sources = []string{runtime.FilesystemSourceName}
	}
	sources := make([]runtime.FilesystemSource, 0, len(request.Sources))
	names := make([]string, 0, len(request.Sources))
	for _, raw := range request.Sources {
		name := strings.TrimSpace(raw)
		source, err := Source(name)
		if err != nil {
			return Prepared{}, fmt.Errorf("invalid request: %w", err)
		}
		sources = append(sources, source)
		// Preserve the caller's existing domain input exactly. The CLI has
		// historically passed its raw flag values, while the Workbench
		// adapter normalizes its request values before calling Prepare.
		names = append(names, raw)
	}
	spec, err := domain.NewObservationSpec(domain.RequestedTarget{Slot: target.Instance.Slot}, names, request.Duration, request.Requester)
	if err != nil {
		return Prepared{}, fmt.Errorf("invalid request: %w", err)
	}
	id, err := domain.NewObservationID()
	if err != nil {
		return Prepared{}, err
	}
	observation, err := domain.NewObservation(id, spec)
	if err != nil {
		return Prepared{}, err
	}
	store, err := obskube.NewStore(clients.Dynamic)
	if err != nil {
		return Prepared{}, err
	}
	if _, err := store.CreateObservation(ctx, request.Namespace, observation); err != nil {
		return Prepared{}, err
	}
	return Prepared{ID: id, Spec: spec, Target: target, Cluster: cluster, Sources: sources, Store: store}, nil
}

func Source(name string) (runtime.FilesystemSource, error) {
	switch name {
	case runtime.FilesystemSourceName:
		return runtime.GadgetFilesystemSource{}, nil
	case "exec":
		return runtime.GadgetExecSource{}, nil
	case "networkConnect":
		return runtime.GadgetNetworkConnectSource{}, nil
	case "networkBind":
		return runtime.GadgetNetworkBindSource{}, nil
	case "capabilities":
		return runtime.GadgetCapabilitiesSource{}, nil
	default:
		return nil, fmt.Errorf("unsupported observation source %q", name)
	}
}
