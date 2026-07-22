// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Part of the landlock-genprof project.

package k8s

import (
	"fmt"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// RestConfig resolves the Kubernetes REST config, trying the in-cluster
// config first (where landlock-genprof will actually run once deployed),
// then falling back to the local kubeconfig — this is what lets the CLI
// (Resolve) and the tracer (which talks to the Inspektor Gadget gRPC
// runtime directly) work identically from a dev machine or in-cluster,
// without duplicating this resolution logic in both places.
func RestConfig() (*rest.Config, error) {
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	kubeconfig := clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
	config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("no cluster configuration found (neither in-cluster nor %s): %w", kubeconfig, err)
	}
	return config, nil
}
