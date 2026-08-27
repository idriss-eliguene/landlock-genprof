package authority

import "fmt"

// SecurityContextIdentity contains security-significant context only.
// Diagnostic Kubernetes/runtime identifiers deliberately do not belong here.
type SecurityContextIdentity struct {
	ImageIdentity      string
	Architecture       string
	ABI                string
	KernelRuntimeClass string
	LibcIdentity       string
	PrivilegeContext   string
	NamespaceSecurity  string
	ConfigurationID    string
	EnvironmentID      string
	FeatureSetID       string
	PersistentStateID  string
	WorkloadIdentity   string
	ExecutableIdentity string
}

func NewSecurityContextIdentity(v SecurityContextIdentity) (SecurityContextIdentity, error) {
	if v.ImageIdentity == "" || v.Architecture == "" || v.ABI == "" ||
		v.KernelRuntimeClass == "" || v.WorkloadIdentity == "" || v.ExecutableIdentity == "" {
		return SecurityContextIdentity{}, fmt.Errorf("security context requires image, architecture, ABI, runtime, workload, and executable identities")
	}
	return v, nil
}
