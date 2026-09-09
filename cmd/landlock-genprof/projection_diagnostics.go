package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	obsdomain "github.com/idriss-eliguene/landlock-genprof/internal/observation/domain"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type projectionStatus string

const (
	projectionHealthy  projectionStatus = "HEALTHY"
	projectionDegraded projectionStatus = "DEGRADED"
)

type projectionDiagnostic struct {
	Kind                string `json:"kind"`
	Namespace           string `json:"namespace,omitempty"`
	Name                string `json:"name,omitempty"`
	UID                 string `json:"uid,omitempty"`
	Category            string `json:"category"`
	Reason              string `json:"reason"`
	Field               string `json:"field,omitempty"`
	ProjectionImpact    string `json:"projectionImpact"`
	SecurityDisposition string `json:"securityDisposition"`
}

type projectionDiagnostics struct {
	Status              projectionStatus       `json:"projectionStatus"`
	ValidObjectCount    int                    `json:"validObjectCount"`
	ExcludedObjectCount int                    `json:"excludedObjectCount"`
	Diagnostics         []projectionDiagnostic `json:"diagnostics,omitempty"`
}

func (d *projectionDiagnostics) addValid() { d.ValidObjectCount++ }

func (d *projectionDiagnostics) addMalformed(objKind, impact string, objMeta objectMetadata, err error) {
	d.Diagnostics = append(d.Diagnostics, newProjectionDiagnostic(objKind, impact, objMeta, err))
	d.ExcludedObjectCount++
	d.Status = projectionDegraded
}

func newProjectionDiagnostic(objKind, impact string, objMeta objectMetadata, err error) projectionDiagnostic {
	return projectionDiagnostic{
		Kind:                objKind,
		Namespace:           objMeta.namespace,
		Name:                objMeta.name,
		UID:                 objMeta.uid,
		Category:            malformedCategory(err),
		Reason:              malformedReason(err),
		Field:               malformedField(err),
		ProjectionImpact:    impact,
		SecurityDisposition: "NOT_ELIGIBLE",
	}
}

func diagnosticForObject(kind string, obj *unstructured.Unstructured, impact string, err error) projectionDiagnostic {
	return newProjectionDiagnostic(kind, impact, metadataOf(obj), err)
}

func (d *projectionDiagnostics) finalize() {
	if d.ExcludedObjectCount == 0 {
		d.Status = projectionHealthy
	}
	sort.Slice(d.Diagnostics, func(i, j int) bool {
		left, right := d.Diagnostics[i], d.Diagnostics[j]
		for _, pair := range [][2]string{{left.Kind, right.Kind}, {left.Namespace, right.Namespace}, {left.Name, right.Name}, {left.UID, right.UID}, {left.Category, right.Category}} {
			if pair[0] != pair[1] {
				return pair[0] < pair[1]
			}
		}
		return left.Field < right.Field
	})
}

func (d projectionDiagnostics) diagnosticFor(kind, namespace, name, uid string) (projectionDiagnostic, bool) {
	for _, diagnostic := range d.Diagnostics {
		if diagnostic.Kind == kind && diagnostic.Namespace == namespace && diagnostic.Name == name && (uid == "" || diagnostic.UID == uid) {
			return diagnostic, true
		}
	}
	return projectionDiagnostic{}, false
}

type objectMetadata struct {
	namespace string
	name      string
	uid       string
}

func metadataOf(obj *unstructured.Unstructured) objectMetadata {
	return objectMetadata{namespace: obj.GetNamespace(), name: obj.GetName(), uid: string(obj.GetUID())}
}

func malformedCategory(err error) string {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unsupported") || strings.Contains(message, "unexpected") {
		return "UNSUPPORTED_VERSION"
	}
	if strings.Contains(message, "invalid spec") || strings.Contains(message, "missing observation spec") || strings.Contains(message, "missing proposal spec") {
		return "SCHEMA_INVALID"
	}
	if strings.Contains(message, "reference") {
		return "REFERENCE_INVALID"
	}
	if errors.Is(err, obsdomain.ErrInvalidDomainValue) {
		if strings.Contains(message, "identity") || strings.Contains(message, "cluster") {
			return "IDENTITY_INVALID"
		}
		if strings.Contains(message, "binding") || strings.Contains(message, "target") || strings.Contains(message, "slot") {
			return "BINDING_INVALID"
		}
		return "DOMAIN_INVALID"
	}
	return "DECODE_ERROR"
}

func malformedReason(err error) string {
	category := malformedCategory(err)
	switch category {
	case "SCHEMA_INVALID":
		return "durable object does not match the expected schema"
	case "UNSUPPORTED_VERSION":
		return "durable object version is not supported by this projection"
	case "REFERENCE_INVALID":
		return "durable object contains an invalid reference"
	case "IDENTITY_INVALID":
		return "durable identity could not be validated"
	case "BINDING_INVALID":
		return "durable target binding could not be validated"
	case "DOMAIN_INVALID":
		return "durable object failed domain validation"
	default:
		return "durable object could not be decoded"
	}
}

func malformedField(err error) string {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "invalid container slot") {
		return "status.binding.resolvedTargets[].imageRevision.slot.workload"
	}
	return ""
}

func (d projectionDiagnostic) String() string {
	if d.Namespace == "" {
		return fmt.Sprintf("%s/%s: %s", d.Kind, d.Name, d.Reason)
	}
	return fmt.Sprintf("%s %s/%s: %s", d.Kind, d.Namespace, d.Name, d.Reason)
}
