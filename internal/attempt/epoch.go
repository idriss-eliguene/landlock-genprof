package attempt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

const CustodyEpochAnnotation = "landlockgenprof.io/custody-epoch"

var CRDGVR = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}

// CurrentCustodyEpoch reads the administrator-published qualification epoch.
// An absent annotation deliberately means legacy/unqualified custody.
func CurrentCustodyEpoch(ctx context.Context, client dynamic.Interface) (string, error) {
	crd, err := getCRD(ctx, client)
	if apierrors.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return crd.GetAnnotations()[CustodyEpochAnnotation], nil
}

func CustodyEpochAndHardening(ctx context.Context, client dynamic.Interface) (string, bool, error) {
	crd, err := getCRD(ctx, client)
	if apierrors.IsNotFound(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	epoch := crd.GetAnnotations()[CustodyEpochAnnotation]
	versions, found, err := unstructured.NestedSlice(crd.Object, "spec", "versions")
	if err != nil || !found || len(versions) == 0 {
		return epoch, false, err
	}
	version, ok := versions[0].(map[string]interface{})
	if !ok {
		return epoch, false, nil
	}
	schemaMap, _, _ := unstructured.NestedMap(version, "schema", "openAPIV3Schema")
	properties, _, _ := unstructured.NestedMap(schemaMap, "properties")
	specMap, _, _ := unstructured.NestedMap(properties, "spec")
	statusMap, _, _ := unstructured.NestedMap(properties, "status")
	specRules, specFound, _ := unstructured.NestedSlice(specMap, "x-kubernetes-validations")
	statusRules, statusFound, _ := unstructured.NestedSlice(statusMap, "x-kubernetes-validations")
	return epoch, specFound && len(specRules) > 0 && statusFound && len(statusRules) > 0, nil
}

func getCRD(ctx context.Context, client dynamic.Interface) (*unstructured.Unstructured, error) {
	return client.Resource(CRDGVR).Get(ctx, "applyattempts.landlockgenprof.io", metav1.GetOptions{})
}

func NewCustodyEpoch() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating custody epoch: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func PatchCustodyEpoch(ctx context.Context, client dynamic.Interface, epoch string) (*unstructured.Unstructured, error) {
	if len(epoch) < 32 {
		return nil, fmt.Errorf("custody epoch must contain at least 128 bits")
	}
	return client.Resource(CRDGVR).Patch(ctx, "applyattempts.landlockgenprof.io", types.MergePatchType, typesMergePatch(epoch), metav1.PatchOptions{}, "")
}

func typesMergePatch(epoch string) []byte {
	return []byte(fmt.Sprintf(`{"metadata":{"annotations":{"%s":%q}}}`, CustodyEpochAnnotation, epoch))
}
