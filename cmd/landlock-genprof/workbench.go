// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/idriss-eliguene/landlock-genprof/internal/authz"
	"github.com/idriss-eliguene/landlock-genprof/internal/k8s"
	"github.com/idriss-eliguene/landlock-genprof/internal/observability"
)

type workbenchOptions struct {
	namespace string
	port      int
}

const workbenchReadHeaderTimeout = 5 * time.Second

func newWorkbenchCmd() *cobra.Command {
	var opts workbenchOptions

	cmd := &cobra.Command{
		Use:   "ui [proposal]",
		Short: "Serves the local read-only Workbench HTTP boundary",
		Long: "Serves the local Workbench: the given SecurityProfileProposal at " +
			"\"/\", plus bounded durable-object workload/security-projection reads under \"/api\". Every read " +
			"goes through the bounded G0.5 read capability; there is no approval, rejection, " +
			"apply, or other mutation control unless authenticated governance mode is enabled." + kubectlPrefixNote,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			proposalName := ""
			if len(args) == 1 {
				proposalName = args[0]
			}
			return runWorkbench(cmd.Context(), cmd.OutOrStdout(), opts, proposalName)
		},
	}
	cmd.Flags().StringVarP(&opts.namespace, "namespace", "n", "default", "Kubernetes namespace")
	cmd.Flags().IntVar(&opts.port, "port", 8080, "Loopback HTTP port")
	return cmd
}

// newWorkbenchReadSession is a test seam, same pattern as this package's
// other newDynamicClientFor* seams. Unlike those, its return type is the
// bounded k8s.WorkbenchReadCapability, not a write-capable client: the
// Workbench HTTP server must not be able to hold one.
var newWorkbenchReadSession = func(namespace string) (k8s.WorkbenchReadCapability, error) {
	config, err := k8s.RestConfig()
	if err != nil {
		return nil, err
	}
	return k8s.NewReadSession(config, namespace)
}

// workbenchShutdownTimeout bounds how long a graceful shutdown waits for
// in-flight requests to drain before the process exits regardless.
const workbenchShutdownTimeout = 5 * time.Second

func runWorkbench(ctx context.Context, stdout io.Writer, opts workbenchOptions, _ string) error {
	if err := validateWorkbenchDeploymentConfig(opts.namespace); err != nil {
		return err
	}
	logger, metrics, obsConfig, err := observability.NewFromEnv(stdout)
	if err != nil {
		return fmt.Errorf("configuring observability: %w", err)
	}
	reads, err := newWorkbenchReadSession(opts.namespace)
	if err != nil {
		return fmt.Errorf("connecting to cluster: %w", err)
	}
	handler, err := newWorkbenchServer(reads, opts.port)
	if err != nil {
		return fmt.Errorf("constructing Workbench server: %w", err)
	}
	handler.logger = logger
	handler.metrics = metrics
	config, err := k8s.RestConfig()
	if err != nil {
		return fmt.Errorf("connecting Workbench observation API: %w", err)
	}
	requestContext, err := enableWorkbenchAuthorization(ctx, config, opts.namespace)
	if err != nil {
		return err
	}
	handler.requestContext = requestContext
	if requestContext == nil {
		writeClient, err := kubernetes.NewForConfig(config)
		if err != nil {
			return fmt.Errorf("constructing Workbench observation client: %w", err)
		}
		dynamicClient, err := dynamic.NewForConfig(config)
		if err != nil {
			return fmt.Errorf("constructing Workbench observation dynamic client: %w", err)
		}
		handler.observations, err = newObservationAPI(writeClient, dynamicClient, opts.namespace)
		if err != nil {
			return fmt.Errorf("constructing Workbench observation API: %w", err)
		}
		// Local demo/qualification mode may compose the same separately
		// deployed Linux executor used by production-like mode. The executor
		// path is startup configuration only; it is never request data and is
		// never exposed by the Workbench HTTP surface.
		if executorPath := strings.TrimSpace(os.Getenv(observationExecutorKubeconfigEnv)); executorPath != "" {
			executorClients, executorErr := authz.NewConfiguredClients(executorPath, strings.TrimSpace(os.Getenv(observationExecutorContextEnv)))
			if executorErr != nil {
				return fmt.Errorf("configuring local Observation executor: %w", executorErr)
			}
			handler.observations = handler.observations.withExecutor(func() (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
				return executorClients.Core, executorClients.Dynamic, executorClients.Config, nil
			})
		}
	}

	addr := workbenchListenAddress(opts.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("binding Workbench listener on %s: %w", addr, err)
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: workbenchReadHeaderTimeout,
		ReadTimeout:       workbenchReadTimeout,
		WriteTimeout:      workbenchWriteTimeout,
		IdleTimeout:       workbenchIdleTimeout,
		MaxHeaderBytes:    workbenchMaxHeaderBytes,
	}
	logger.Info("listener_ready", map[string]interface{}{"component": "operations_center", "deployment_mode": os.Getenv(workbenchDeploymentModeEnv), "address": addr})
	fmt.Fprintf(stdout, "Local Workbench: http://%s\n", addr)
	fmt.Fprintln(stdout, "Governance mutations require authenticated Operations Center mode.")
	handler.lifecycle.markStarted()
	var metricsServer *http.Server
	var metricsListener net.Listener
	if obsConfig.Metrics {
		metricsListener, err = net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", obsConfig.MetricsPort))
		if err != nil {
			_ = listener.Close()
			return fmt.Errorf("binding metrics listener: %w", err)
		}
		metricsServer = &http.Server{Handler: http.HandlerFunc(metrics.ServeHTTP), ReadHeaderTimeout: workbenchReadHeaderTimeout, ReadTimeout: workbenchReadTimeout, WriteTimeout: workbenchWriteTimeout}
		go func() { _ = metricsServer.Serve(metricsListener) }()
		logger.Info("metrics_listener_ready", map[string]interface{}{"component": "operations_center", "port": obsConfig.MetricsPort})
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	select {
	case err := <-serveErr:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("Workbench listener: %w", err)
		}
		return nil
	case <-ctx.Done():
		logger.Info("shutdown_initiated", map[string]interface{}{"component": "operations_center"})
		handler.lifecycle.beginDrain()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), workbenchShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down Workbench: %w", err)
		}
		if metricsServer != nil {
			_ = metricsServer.Shutdown(context.Background())
		}
		<-serveErr
		logger.Info("shutdown_completed", map[string]interface{}{"component": "operations_center"})
		return nil
	}
}

func workbenchListenAddress(port int) string {
	if strings.EqualFold(strings.TrimSpace(os.Getenv(workbenchDeploymentModeEnv)), "production") {
		return fmt.Sprintf("0.0.0.0:%d", port)
	}
	return fmt.Sprintf("127.0.0.1:%d", port)
}

func workbenchAllowedHost(port int) string {
	if strings.EqualFold(strings.TrimSpace(os.Getenv(workbenchDeploymentModeEnv)), "production") {
		if host := strings.TrimSpace(os.Getenv(workbenchAllowedHostEnv)); host != "" {
			return host
		}
	}
	return fmt.Sprintf("127.0.0.1:%d", port)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
