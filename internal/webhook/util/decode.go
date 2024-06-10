package util

import (
	"context"
	"fmt"

	druiderr "github.com/gardener/etcd-druid/internal/errors"
	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/scale/scheme/autoscalingv1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	// ErrDecodeRequestObject indicates an error in decoding the request object.
	ErrDecodeRequestObject = "ERR_DECODE_REQUEST_OBJECT"
	// ErrGetStatefulSet indicates an error in fetching the StatefulSet resource.
	ErrGetStatefulSet = "ERR_GET_SCALE_SUBRESOURCE_PARENT"
	// ErrTooManyMatchingStatefulSets indicates that more than one StatefulSet was found for the given labels.
	ErrTooManyMatchingStatefulSets = "ERR_TOO_MANY_MATCHING_STATEFULSETS"
)

// RequestDecoder is a decoder for admission requests.
type RequestDecoder struct {
	decoder *admission.Decoder
	client  client.Client
}

// NewRequestDecoder returns a new RequestDecoder.
func NewRequestDecoder(mgr manager.Manager) *RequestDecoder {
	return &RequestDecoder{
		decoder: admission.NewDecoder(mgr.GetScheme()),
		client:  mgr.GetClient(),
	}
}

// DecodeRequestObjectAsPartialObjectMetadata decodes the request object as a PartialObjectMetadata.
func (d *RequestDecoder) DecodeRequestObjectAsPartialObjectMetadata(ctx context.Context, req admission.Request) (*metav1.PartialObjectMetadata, error) {
	var (
		err            error
		partialObjMeta *metav1.PartialObjectMetadata
	)
	requestGK := schema.GroupKind{Group: req.Kind.Group, Kind: req.Kind.Kind}
	switch requestGK {
	case
		corev1.SchemeGroupVersion.WithKind("ServiceAccount").GroupKind(),
		corev1.SchemeGroupVersion.WithKind("Service").GroupKind(),
		corev1.SchemeGroupVersion.WithKind("ConfigMap").GroupKind(),
		rbacv1.SchemeGroupVersion.WithKind("Role").GroupKind(),
		rbacv1.SchemeGroupVersion.WithKind("RoleBinding").GroupKind(),
		appsv1.SchemeGroupVersion.WithKind("StatefulSet").GroupKind(),
		policyv1.SchemeGroupVersion.WithKind("PodDisruptionBudget").GroupKind(),
		batchv1.SchemeGroupVersion.WithKind("Job").GroupKind(),
		coordinationv1.SchemeGroupVersion.WithKind("Lease").GroupKind():
		return d.doDecodeRequestObjAsPartialObjectMeta(req)
	case autoscalingv1.SchemeGroupVersion.WithKind("Scale").GroupKind():
		return d.getStatefulSetPartialObjMetaFromScaleSubResource(ctx, req)
	case corev1.SchemeGroupVersion.WithKind("PersistentVolumeClaim").GroupKind():
		return d.getStatefulSetPartialObjMetaFromPVC(ctx, req)
	}
	return partialObjMeta, err
}

func (d *RequestDecoder) doDecodeRequestObjAsPartialObjectMeta(req admission.Request) (*metav1.PartialObjectMetadata, error) {
	oldObj, newObj, err := d.doDecodeRequestObjects(req)
	if err != nil {
		return nil, err
	}
	if req.Operation == admissionv1.Delete {
		return meta.AsPartialObjectMetadata(oldObj), err
	}
	return meta.AsPartialObjectMetadata(newObj), err
}

func (d *RequestDecoder) doDecodeRequestObjects(req admission.Request) (oldObj, newObj *unstructured.Unstructured, err error) {
	gk := GetGroupKindFromRequest(req)
	switch req.Operation {
	case admissionv1.Connect:
		return
	case admissionv1.Create:
		newObj, err = d.decodeObjectAsUnstructured(gk, req.Object)
		return
	case admissionv1.Delete:
		// OldObject contains the object being deleted
		//https://github.com/kubernetes/kubernetes/pull/76346
		oldObj, err = d.decodeObjectAsUnstructured(gk, req.OldObject)
		return
	case admissionv1.Update:
		if newObj, err = d.decodeObjectAsUnstructured(gk, req.Object); err != nil {
			return
		}
		if oldObj, err = d.decodeObjectAsUnstructured(gk, req.OldObject); err != nil {
			return
		}
	default:
		err = druiderr.WrapError(fmt.Errorf("unsupported operation %s", req.Operation), ErrDecodeRequestObject, "doDecodeRequestObjects", "unsupported operation")
	}
	return
}

func (d *RequestDecoder) decodeObjectAsUnstructured(gk schema.GroupKind, rawObj runtime.RawExtension) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	if err := d.decoder.DecodeRaw(rawObj, obj); err != nil {
		return nil, druiderr.WrapError(err, ErrDecodeRequestObject, "decodeObjectAsUnstructured", fmt.Sprintf("failed to decode object: %v", gk))
	}
	return obj, nil
}

func (d *RequestDecoder) getStatefulSetPartialObjMetaFromScaleSubResource(ctx context.Context, req admission.Request) (*metav1.PartialObjectMetadata, error) {
	requestResourceGK := schema.GroupResource{Group: req.Resource.Group, Resource: req.Resource.Resource}
	if requestResourceGK == appsv1.SchemeGroupVersion.WithResource("statefulsets").GroupResource() {
		objMeta := &metav1.PartialObjectMetadata{}
		objMeta.SetGroupVersionKind(appsv1.SchemeGroupVersion.WithKind("StatefulSet"))
		objKey := client.ObjectKey{Name: req.Name, Namespace: req.Namespace}
		if err := d.client.Get(ctx, objKey, objMeta); err != nil {
			return nil, druiderr.WrapError(err, ErrGetStatefulSet, "GetStatefulSetPartialObjectMetaFromScaleSubResource", fmt.Sprintf("failed to fetch StatefulSet for scale subresource: %v", objKey))
		}
		return objMeta, nil
	}
	return nil, nil
}

func (d *RequestDecoder) getStatefulSetPartialObjMetaFromPVC(ctx context.Context, req admission.Request) (*metav1.PartialObjectMetadata, error) {
	obj, err := d.doDecodeRequestObjAsPartialObjectMeta(req)
	if err != nil {
		return nil, err
	}
	labels := obj.GetLabels()
	if len(labels) == 0 {
		return nil, &druiderr.DruidError{
			Code:      ErrGetStatefulSet,
			Operation: "GetStatefulSetPartialObjMetaFromPVC",
			Message:   fmt.Sprintf("resource %s/%s does not have any labels", req.Namespace, req.Name),
		}
	}

	objMetaList := &metav1.PartialObjectMetadataList{}
	objMetaList.SetGroupVersionKind(appsv1.SchemeGroupVersion.WithKind("StatefulSet"))
	if err = d.client.List(ctx, objMetaList, client.InNamespace(req.Namespace), client.MatchingLabels(labels)); err != nil {
		return nil, druiderr.WrapError(err, ErrGetStatefulSet, "GetStatefulSetPartialObjMetaFromPVC", fmt.Sprintf("failed to fetch StatefulSet for labels: %v", labels))
	}
	if len(objMetaList.Items) > 1 {
		return nil, &druiderr.DruidError{
			Code:      ErrTooManyMatchingStatefulSets,
			Operation: "FetchStatefulSetByLabels",
			Message:   fmt.Sprintf("found more than one StatefulSet for labels: %v", labels),
		}
	}
	if len(objMetaList.Items) == 0 {
		return nil, nil
	}
	return &objMetaList.Items[0], nil
}
