// Copyright (c) 2026 Idriss ELIGUENE
// Author: Idriss ELIGUENE <idriss.eliguene@gmail.com>
// SPDX-License-Identifier: Apache-2.0
//
// Part of the landlock-genprof project.

// Package k8s localise et prépare le pod cible d'un training run
// (résolution du namespace/pod/container, vérification des permissions
// RBAC nécessaires au tracer).
package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// TargetPod identifie le pod/conteneur à observer.
type TargetPod struct {
	Namespace string
	PodName   string
	Container string
}

// Resolve vérifie que le pod cible existe, tourne, et que le conteneur
// demandé y est présent (ou se déduit s'il n'y en a qu'un). Le client est
// injecté plutôt que construit ici, pour permettre les tests avec le
// clientset factice de client-go (k8s.io/client-go/kubernetes/fake) sans
// dépendre d'un vrai cluster — voir internal/k8s/target_test.go.
//
// RBAC minimal requis pour ce seul appel : `get` sur `pods` dans le
// namespace ciblé. Voir docs/threat-model.md pour le RBAC complet du
// tracer (au-delà de cette résolution).
func Resolve(ctx context.Context, client kubernetes.Interface, namespace, podName, container string) (*TargetPod, error) {
	pod, err := client.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("pod %s/%s introuvable: %w", namespace, podName, err)
	}
	if err != nil {
		return nil, fmt.Errorf("résolution du pod %s/%s: %w", namespace, podName, err)
	}

	if pod.Status.Phase != corev1.PodRunning {
		return nil, fmt.Errorf("pod %s/%s n'est pas en cours d'exécution (phase actuelle: %s)",
			namespace, podName, pod.Status.Phase)
	}

	resolvedContainer, err := resolveContainer(pod, container)
	if err != nil {
		return nil, err
	}

	return &TargetPod{
		Namespace: namespace,
		PodName:   podName,
		Container: resolvedContainer,
	}, nil
}

// resolveContainer valide le conteneur demandé, ou le déduit si le pod
// n'en a qu'un seul (comportement calqué sur `kubectl exec` sans --container).
func resolveContainer(pod *corev1.Pod, container string) (string, error) {
	if len(pod.Spec.Containers) == 0 {
		return "", fmt.Errorf("pod %s/%s n'a aucun conteneur", pod.Namespace, pod.Name)
	}

	if container == "" {
		if len(pod.Spec.Containers) > 1 {
			return "", fmt.Errorf(
				"pod %s/%s a plusieurs conteneurs (%d) : précise --container",
				pod.Namespace, pod.Name, len(pod.Spec.Containers),
			)
		}
		return pod.Spec.Containers[0].Name, nil
	}

	for _, c := range pod.Spec.Containers {
		if c.Name == container {
			return container, nil
		}
	}
	return "", fmt.Errorf("conteneur %q introuvable dans le pod %s/%s", container, pod.Namespace, pod.Name)
}
