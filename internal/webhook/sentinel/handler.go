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
	"k8s.io/apiserver/pkg/authentication/serviceaccount"
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
	log.V(1).Info("Sentinel webhook invoked")

	if slices.Contains(allowedOperations, req.Operation) {
		return admission.Allowed(fmt.Sprintf("operation %s is allowed", req.Operation))
	}

	etcd, allowedMessage, err := h.getEtcdForRequest(ctx, req)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if allowedMessage != "" {
		return admission.Allowed(allowedMessage)
	}

	// allow deletion operation on resources if the Etcd is currently being deleted, but only by etcd-druid and exempt service accounts.
	if req.Operation == admissionv1.Delete && etcd.IsDeletionInProgress() {
		if req.UserInfo.Username == h.config.ReconcilerServiceAccount {
			return admission.Allowed(fmt.Sprintf("deletion of resource by etcd-druid is allowed during deletion of Etcd %s", etcd.Name))
		}
		if slices.Contains(h.config.ExemptServiceAccounts, req.UserInfo.Username) {
			return admission.Allowed(fmt.Sprintf("deletion of resource by exempt SA %s is allowed during deletion of Etcd %s", req.UserInfo.Username, etcd.Name))
		}
		return admission.Denied(fmt.Sprintf("no external intervention allowed during ongoing deletion of Etcd %s by etcd-druid", etcd.Name))
	}

	// Leases (member and snapshot) will be periodically updated by etcd members.
	// Allow updates to such leases, but only by etcd members, which would use the serviceaccount deployed by druid for them.
	requestGK := schema.GroupKind{Group: req.Kind.Group, Kind: req.Kind.Kind}
	if requestGK == coordinationv1.SchemeGroupVersion.WithKind("Lease").GroupKind() && req.Operation == admissionv1.Update {
		if serviceaccount.MatchesUsername(etcd.GetNamespace(), druidv1alpha1.GetServiceAccountName(etcd.ObjectMeta), req.UserInfo.Username) {
			return admission.Allowed("lease resource can be freely updated by etcd members")
		}
	}

	// allow changes to resources if Etcd has annotation druid.gardener.cloud/disable-resource-protection is set.
	if !druidv1alpha1.AreManagedResourcesProtected(etcd.ObjectMeta) {
		return admission.Allowed(fmt.Sprintf("changes allowed, since Etcd %s has annotation %s", etcd.Name, druidv1alpha1.DisableResourceProtectionAnnotation))
	}

	// allow operations on resources if the Etcd is currently being reconciled, but only by etcd-druid.
	if etcd.IsReconciliationInProgress() {
		if req.UserInfo.Username == h.config.ReconcilerServiceAccount {
			return admission.Allowed(fmt.Sprintf("ongoing reconciliation of Etcd %s by etcd-druid requires changes to resources", etcd.Name))
		}
		return admission.Denied(fmt.Sprintf("no external intervention allowed during ongoing reconciliation of Etcd %s by etcd-druid", etcd.Name))
	}

	// allow exempt service accounts to make changes to resources, but only if the Etcd is not currently being reconciled.
	for _, sa := range h.config.ExemptServiceAccounts {
		if req.UserInfo.Username == sa {
			return admission.Allowed(fmt.Sprintf("operations on Etcd %s by service account %s is exempt from Sentinel Webhook checks", etcd.Name, sa))
		}
	}

	return admission.Denied(fmt.Sprintf("changes disallowed, since no ongoing processing of Etcd %s by etcd-druid", etcd.Name))
}

// getEtcdForRequest returns the Etcd resource corresponding to the object in the admission request.
// It also returns admission response message and error, if any.
func (h *Handler) getEtcdForRequest(ctx context.Context, req admission.Request) (*druidv1alpha1.Etcd, string, error) {
	obj, err := h.decodeRequestObject(ctx, req)
	if err != nil {
		return nil, "", err
	}
	if obj == nil {
		return nil, fmt.Sprintf("unexpected resource type: %s/%s", req.Kind.Group, req.Kind.Kind), nil
	}

	// allow changes for resources not managed by etcd-druid
	managedBy, hasLabel := obj.GetLabels()[druidv1alpha1.LabelManagedByKey]
	if !hasLabel || managedBy != druidv1alpha1.LabelManagedByValue {
		return nil, fmt.Sprintf("resource is not managed by etcd-druid, as label %s is missing", druidv1alpha1.LabelManagedByKey), nil
	}

	// get the name of the Etcd that the resource is part of
	etcdName, hasLabel := obj.GetLabels()[druidv1alpha1.LabelPartOfKey]
	if !hasLabel {
		return nil, fmt.Sprintf("label %s not found on resource", druidv1alpha1.LabelPartOfKey), nil
	}

	etcd := &druidv1alpha1.Etcd{}
	if err = h.Get(ctx, types.NamespacedName{Name: etcdName, Namespace: req.Namespace}, etcd); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Sprintf("corresponding Etcd %s not found", etcdName), nil
		}
		return nil, "", err
	}

	return etcd, "", nil

}

// decodeRequestObject decodes the relevant object from the admission request and returns it.
// If it encounters an unexpected resource type, it returns a nil object.
func (h *Handler) decodeRequestObject(ctx context.Context, req admission.Request) (client.Object, error) {
	var (
		requestGK = schema.GroupKind{Group: req.Kind.Group, Kind: req.Kind.Kind}
		obj       client.Object
		err       error
	)
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
		obj, err = h.doDecodeRequestObject(req)

	// if admission request is for statefulsets/scale subresource, then fetch and return the parent statefulset object instead.
	case autoscalingv1.SchemeGroupVersion.WithKind("Scale").GroupKind():
		requestResourceGK := schema.GroupResource{Group: req.Resource.Group, Resource: req.Resource.Resource}
		if requestResourceGK == appsv1.SchemeGroupVersion.WithResource("statefulsets").GroupResource() {
			obj, err = h.fetchStatefulSet(ctx, req.Name, req.Namespace)
		}
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
