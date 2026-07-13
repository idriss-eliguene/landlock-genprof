// Package policy agrège des événements de tracing (internal/tracer) en un
// profil Landlock minimal, au format compatible avec le CRD LandlockProfile
// de PodLock (voir pkg/podlock).
package policy

import "github.com/mekonbot/landlock-genprof/internal/tracer"

// Confidence indique le niveau de certitude d'une règle générée, en
// fonction du nombre de training runs où elle a été observée.
type Confidence string

const (
	ConfidenceLow    Confidence = "low"    // vu une seule fois
	ConfidenceMedium Confidence = "medium" // vu sur plusieurs runs, incohérent
	ConfidenceHigh   Confidence = "high"   // vu systématiquement
)

// Rule est une règle candidate avant export au format PodLock.
type Rule struct {
	Path       string
	Access     []string // ex: "readExec", "readOnly", "readWrite"
	Confidence Confidence
	SeenCount  int
}

// Synthesize agrège une liste d'événements (potentiellement issus de
// plusieurs training runs) en un ensemble de règles minimales.
//
// TODO(M2, Étudiant B): implémenter l'agrégation par répertoire (éviter
// le sur-fitting fichier-par-fichier) et le calcul de confiance.
//
// Point d'attention méthodologique (Étudiante C): un training run court
// ne couvre pas les chemins de code rares (erreurs, cas limites). Le champ
// Confidence doit rendre ce risque de faux négatif visible dans le YAML
// généré, pas le cacher derrière une policy qui a l'air complète.
func Synthesize(events []tracer.Event) ([]Rule, error) {
	panic("not implemented")
}
