package util

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// GetGroupKindAsStringFromRequest returns the GroupKind as a string from the given admission request.
func GetGroupKindAsStringFromRequest(req admission.Request) string {
	return fmt.Sprintf("%s/%s", req.Kind.Group, req.Kind.Kind)
}

// GetGroupKindFromRequest returns the GroupKind from the given admission request.
func GetGroupKindFromRequest(req admission.Request) schema.GroupKind {
	return schema.GroupKind{Group: req.Kind.Group, Kind: req.Kind.Kind}
}

// CreateObjectKey creates a client.ObjectKey from the given PartialObjectMetadata.
func CreateObjectKey(partialObjMeta *metav1.PartialObjectMetadata) client.ObjectKey {
	return client.ObjectKey{
		Namespace: partialObjMeta.Namespace,
		Name:      partialObjMeta.Name,
	}
}
