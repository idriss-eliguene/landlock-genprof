package domain

import (
	"fmt"
	"sort"
	"strings"

	"github.com/idriss-eliguene/landlock-genprof/internal/profile"
)

const maxNormalizedFacts = 256

// FilesystemFact is one deduplicated positive filesystem observation.
type FilesystemFact struct {
	Path        string
	Permissions []profile.FilePermission
}

// NetworkFact is one deduplicated positive network observation.
type NetworkFact struct {
	Port      int
	Direction profile.NetworkDirection
}

// CapabilityFact is one deduplicated positive capability observation.
type CapabilityFact struct{ Name string }

// ExecFact is retained as Observation evidence. It is intentionally not a
// syscall fact and has no current TrainingHistory compatibility.
type ExecFact struct{ Path string }

// NormalizedFacts is the closed, bounded positive-fact vocabulary for an
// Observation source. Only the field matching the source is populated.
type NormalizedFacts struct {
	Filesystem     []FilesystemFact
	Exec           []ExecFact
	NetworkConnect []NetworkFact
	NetworkBind    []NetworkFact
	Capabilities   []CapabilityFact
}

func validatePermissions(values []profile.FilePermission) bool {
	if len(values) == 0 {
		return false
	}
	seen := map[profile.FilePermission]bool{}
	for _, value := range values {
		switch value {
		case profile.PermissionRead, profile.PermissionWrite, profile.PermissionExecute:
		default:
			return false
		}
		if seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func (f NormalizedFacts) ValidateForSource(source string) error {
	counts := []int{len(f.Filesystem), len(f.Exec), len(f.NetworkConnect), len(f.NetworkBind), len(f.Capabilities)}
	for _, count := range counts {
		if count > maxNormalizedFacts {
			return fmt.Errorf("%w: normalized fact limit exceeded", ErrInvalidDomainValue)
		}
	}
	// Existing observations may use a generic source label such as "network"
	// without typed facts. Preserve that readable legacy shape; any non-empty
	// typed fact set still requires one of the closed source names below.
	if f.Count() == 0 {
		return nil
	}
	if source == "filesystem" {
		if len(f.Exec)+len(f.NetworkConnect)+len(f.NetworkBind)+len(f.Capabilities) != 0 {
			return fmt.Errorf("%w: facts do not match filesystem source", ErrInvalidDomainValue)
		}
		seen := map[string]bool{}
		for _, fact := range f.Filesystem {
			if strings.TrimSpace(fact.Path) == "" || !validatePermissions(fact.Permissions) || seen[fact.Path] {
				return fmt.Errorf("%w: invalid filesystem facts", ErrInvalidDomainValue)
			}
			seen[fact.Path] = true
		}
		return nil
	}
	if source == "exec" {
		if len(f.Filesystem)+len(f.NetworkConnect)+len(f.NetworkBind)+len(f.Capabilities) != 0 {
			return fmt.Errorf("%w: facts do not match exec source", ErrInvalidDomainValue)
		}
		seen := map[string]bool{}
		for _, fact := range f.Exec {
			if strings.TrimSpace(fact.Path) == "" || seen[fact.Path] {
				return fmt.Errorf("%w: invalid exec facts", ErrInvalidDomainValue)
			}
			seen[fact.Path] = true
		}
		return nil
	}
	if source == "networkConnect" || source == "networkBind" {
		if len(f.Filesystem)+len(f.Exec)+len(f.Capabilities) != 0 {
			return fmt.Errorf("%w: facts do not match network source", ErrInvalidDomainValue)
		}
		facts := f.NetworkConnect
		if source == "networkBind" {
			facts = f.NetworkBind
		}
		if source == "networkConnect" && len(f.NetworkBind) != 0 || source == "networkBind" && len(f.NetworkConnect) != 0 {
			return fmt.Errorf("%w: network facts do not match source", ErrInvalidDomainValue)
		}
		seen := map[string]bool{}
		for _, fact := range facts {
			if fact.Port <= 0 || (fact.Direction != profile.DirectionEgress && fact.Direction != profile.DirectionIngress) {
				return fmt.Errorf("%w: invalid network facts", ErrInvalidDomainValue)
			}
			key := fmt.Sprintf("%d/%s", fact.Port, fact.Direction)
			if seen[key] {
				return fmt.Errorf("%w: duplicate network facts", ErrInvalidDomainValue)
			}
			seen[key] = true
		}
		return nil
	}
	if source == "capabilities" {
		if len(f.Filesystem)+len(f.Exec)+len(f.NetworkConnect)+len(f.NetworkBind) != 0 {
			return fmt.Errorf("%w: facts do not match capability source", ErrInvalidDomainValue)
		}
		seen := map[string]bool{}
		for _, fact := range f.Capabilities {
			if strings.TrimSpace(fact.Name) == "" || len(fact.Name) > 128 || seen[fact.Name] {
				return fmt.Errorf("%w: invalid capability facts", ErrInvalidDomainValue)
			}
			seen[fact.Name] = true
		}
		return nil
	}
	return fmt.Errorf("%w: unsupported source %q", ErrInvalidDomainValue, source)
}

func (f NormalizedFacts) Count() int {
	return len(f.Filesystem) + len(f.Exec) + len(f.NetworkConnect) + len(f.NetworkBind) + len(f.Capabilities)
}

func (f NormalizedFacts) Copy() NormalizedFacts {
	out := f
	out.Filesystem = append([]FilesystemFact(nil), f.Filesystem...)
	out.Exec = append([]ExecFact(nil), f.Exec...)
	out.NetworkConnect = append([]NetworkFact(nil), f.NetworkConnect...)
	out.NetworkBind = append([]NetworkFact(nil), f.NetworkBind...)
	out.Capabilities = append([]CapabilityFact(nil), f.Capabilities...)
	for i := range out.Filesystem {
		out.Filesystem[i].Permissions = append([]profile.FilePermission(nil), out.Filesystem[i].Permissions...)
	}
	return out
}

func sortFacts(f *NormalizedFacts) {
	sort.Slice(f.Filesystem, func(i, j int) bool { return f.Filesystem[i].Path < f.Filesystem[j].Path })
	for i := range f.Filesystem {
		sort.Slice(f.Filesystem[i].Permissions, func(a, b int) bool { return f.Filesystem[i].Permissions[a] < f.Filesystem[i].Permissions[b] })
	}
	sort.Slice(f.Exec, func(i, j int) bool { return f.Exec[i].Path < f.Exec[j].Path })
	sort.Slice(f.NetworkConnect, func(i, j int) bool {
		if f.NetworkConnect[i].Port != f.NetworkConnect[j].Port {
			return f.NetworkConnect[i].Port < f.NetworkConnect[j].Port
		}
		return f.NetworkConnect[i].Direction < f.NetworkConnect[j].Direction
	})
	sort.Slice(f.NetworkBind, func(i, j int) bool {
		if f.NetworkBind[i].Port != f.NetworkBind[j].Port {
			return f.NetworkBind[i].Port < f.NetworkBind[j].Port
		}
		return f.NetworkBind[i].Direction < f.NetworkBind[j].Direction
	})
	sort.Slice(f.Capabilities, func(i, j int) bool { return f.Capabilities[i].Name < f.Capabilities[j].Name })
}
