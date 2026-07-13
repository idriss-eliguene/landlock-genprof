// Package k8s localise et prépare le pod cible d'un training run
// (résolution du namespace/pod/container, vérification des permissions
// RBAC nécessaires au tracer).
package k8s

// TargetPod identifie le pod/conteneur à observer.
type TargetPod struct {
	Namespace string
	PodName   string
	Container string
}

// Resolve vérifie que le pod cible existe et que le tracer dispose des
// permissions nécessaires pour l'observer.
//
// TODO(M1, Étudiant B): implémenter via client-go. Voir docs/threat-model.md
// pour le RBAC minimal requis (namespace dédié, pas de privilèges cluster-wide
// au-delà de ce qui est strictement nécessaire au tracer).
func Resolve(namespace, pod, container string) (*TargetPod, error) {
	panic("not implemented")
}
