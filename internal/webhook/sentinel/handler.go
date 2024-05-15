// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package sentinel

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"

	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/scale/scheme/autoscalingv1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var allowedOperations = []admissionv1.Operation{admissionv1.Create, admissionv1.Connect}

// Handler is the Sentinel Webhook admission handler.
type Handler struct {
	client.Client
	config  *Config
	decoder *admission.Decoder
	logger  logr.Logger
}

// NewHandler creates a new handler for Sentinel Webhook.
func NewHandler(mgr manager.Manager, config *Config) (*Handler, error) {
	decoder := admission.NewDecoder(mgr.GetScheme())
	return &Handler{
		Client:  mgr.GetClient(),
		config:  config,
		decoder: decoder,
		logger:  mgr.GetLogger().WithName(handlerName),
	}, nil
}

// Handle handles admission requests and prevents unintended changes to resources created by etcd-druid.
func (h *Handler) Handle(ctx context.Context, req admission.Request) admission.Response {
	requestGKString := fmt.Sprintf("%s/%s", req.Kind.Group, req.Kind.Kind)
	log := h.logger.WithValues("name", req.Name, "namespace", req.Namespace, "resourceGroupKind", requestGKString, "operation", req.Operation, "user", req.UserInfo.Username)
	log.Info("Sentinel webhook invoked")

	if slices.Contains(allowedOperations, req.Operation) {
		return admission.Allowed(fmt.Sprintf("operation %s is allowed", req.Operation))
	}

	requestGK := schema.GroupKind{Group: req.Kind.Group, Kind: req.Kind.Kind}

	// Leases (member and snapshot) will be periodically updated by etcd members. Allow updates to leases.
	if requestGK == coordinationv1.SchemeGroupVersion.WithKind("Lease").GroupKind() &&
		req.Operation == admissionv1.Update {
		return admission.Allowed("lease resource can be freely updated")
	}

	obj, err := h.decodeRequestObject(req, requestGK)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if obj == nil {
		return admission.Allowed(fmt.Sprintf("unexpected resource type: %s", requestGKString))
	}

	// If admission request is for statefulsets/scale subresource, then check labels on the parent statefulset object
	// and allow the request if it doesn't contain label `app.kubernetes.io/managed-by: etcd-druid`.
	// This special handling is required for statefulsets/scale subresource, since it does not work when
	// an objectSelector is specified for it in the ValidatingWebhookConfiguration.
	// More information can be found at https://github.com/kubernetes/kubernetes/issues/113594#issuecomment-1332573990
	requestResourceGK := schema.GroupResource{Group: req.Resource.Group, Resource: req.Resource.Resource}
	if requestGK == autoscalingv1.SchemeGroupVersion.WithKind("Scale").GroupKind() &&
		requestResourceGK == appsv1.SchemeGroupVersion.WithResource("statefulsets").GroupResource() {
		sts, err := h.fetchStatefulSet(ctx, req.Name, req.Namespace)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
		managedBy, hasLabel := sts.GetLabels()[druidv1alpha1.LabelManagedByKey]
		if !hasLabel || managedBy != druidv1alpha1.LabelManagedByValue {
			return admission.Allowed(fmt.Sprintf("label %s not found on resource", druidv1alpha1.LabelPartOfKey))
		}
		// replicate sts labels to the /scale subresource object, used for subsequent checks
		obj.SetLabels(sts.GetLabels())
	}

	etcdName, hasLabel := obj.GetLabels()[druidv1alpha1.LabelPartOfKey]
	if !hasLabel {
		return admission.Allowed(fmt.Sprintf("label %s not found on resource", druidv1alpha1.LabelPartOfKey))
	}

	etcd := &druidv1alpha1.Etcd{}
	if err = h.Get(ctx, types.NamespacedName{Name: etcdName, Namespace: req.Namespace}, etcd); err != nil {
		if apierrors.IsNotFound(err) {
			return admission.Allowed(fmt.Sprintf("corresponding Etcd %s not found", etcdName))
		}
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// allow changes to resources if Etcd has annotation druid.gardener.cloud/disable-resource-protection is set
	if !etcd.AreManagedResourcesProtected() {
		return admission.Allowed(fmt.Sprintf("changes allowed, since Etcd %s has annotation %s", etcd.Name, druidv1alpha1.DisableResourceProtectionAnnotation))
	}

	// allow operations on resources if the Etcd is currently being reconciled, but only by etcd-druid,
	// and allow exempt service accounts to make changes to resources, but only if the Etcd is not currently being reconciled.
	if etcd.IsReconciliationInProgress() {
		if req.UserInfo.Username == h.config.ReconcilerServiceAccount {
			return admission.Allowed(fmt.Sprintf("ongoing reconciliation of Etcd %s by etcd-druid requires changes to resources", etcd.Name))
		}
		return admission.Denied(fmt.Sprintf("no external intervention allowed during ongoing reconciliation of Etcd %s by etcd-druid", etcd.Name))
	} else {
		for _, sa := range h.config.ExemptServiceAccounts {
			if req.UserInfo.Username == sa {
				return admission.Allowed(fmt.Sprintf("operations on Etcd %s by service account %s is exempt from Sentinel Webhook checks", etcd.Name, sa))
			}
		}
	}

	return admission.Denied(fmt.Sprintf("changes disallowed, since no ongoing processing of Etcd %s by etcd-druid", etcd.Name))
}

func (h *Handler) decodeRequestObject(req admission.Request, requestGK schema.GroupKind) (client.Object, error) {
	var (
		obj client.Object
		err error
	)
	switch requestGK {
	case
		corev1.SchemeGroupVersion.WithKind("ServiceAccount").GroupKind(),
		corev1.SchemeGroupVersion.WithKind("Service").GroupKind(),
		corev1.SchemeGroupVersion.WithKind("ConfigMap").GroupKind(),
		rbacv1.SchemeGroupVersion.WithKind("Role").GroupKind(),
		rbacv1.SchemeGroupVersion.WithKind("RoleBinding").GroupKind(),
		appsv1.SchemeGroupVersion.WithKind("StatefulSet").GroupKind(),
		autoscalingv1.SchemeGroupVersion.WithKind("Scale").GroupKind(), // for statefulsets' /scale subresource
		policyv1.SchemeGroupVersion.WithKind("PodDisruptionBudget").GroupKind(),
		batchv1.SchemeGroupVersion.WithKind("Job").GroupKind(),
		coordinationv1.SchemeGroupVersion.WithKind("Lease").GroupKind():
		obj, err = h.doDecodeRequestObject(req)
	}
	return obj, err
}

func (h *Handler) doDecodeRequestObject(req admission.Request) (client.Object, error) {
	var (
		err error
		obj = &unstructured.Unstructured{}
	)

	if req.Operation == admissionv1.Delete {
		if err = h.decoder.DecodeRaw(req.OldObject, obj); err != nil {
			return nil, err
		}
		return obj, nil
	}

	if err = h.decoder.Decode(req, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func (h *Handler) fetchStatefulSet(ctx context.Context, name, namespace string) (*appsv1.StatefulSet, error) {
	var (
		err error
		sts = &appsv1.StatefulSet{}
	)

	if err = h.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, sts); err != nil {
		return nil, err
	}
	return sts, nil
}
