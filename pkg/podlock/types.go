// Package podlock définit les types Go correspondant au schéma CRD
// LandlockProfile du projet PodLock (github.com/flavio/podlock,
// écosystème Kubewarden), afin que landlock-genprof génère des profils
// directement utilisables sans transformation supplémentaire.
//
// TODO(M2): valider ces types face au schéma réel de PodLock au moment
// de l'implémentation (le format peut évoluer) — voir
// https://github.com/flavio/podlock
package podlock

// LandlockProfile miroir du CRD PodLock.
type LandlockProfile struct {
	APIVersion string               `yaml:"apiVersion"`
	Kind       string               `yaml:"kind"`
	Metadata   Metadata             `yaml:"metadata"`
	Spec       LandlockProfileSpec  `yaml:"spec"`
}

type Metadata struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}

type LandlockProfileSpec struct {
	// ProfilesByContainer: nom du conteneur -> chemin binaire -> restrictions
	ProfilesByContainer map[string]map[string]BinaryProfile `yaml:"profilesByContainer"`
}

type BinaryProfile struct {
	ReadExec  []string `yaml:"readExec,omitempty"`
	ReadOnly  []string `yaml:"readOnly,omitempty"`
	ReadWrite []string `yaml:"readWrite,omitempty"`
}
