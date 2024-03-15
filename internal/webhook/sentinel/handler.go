// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package sentinel

import (
	"context"
	"fmt"
	"net/http"

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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Handler is the Sentinel Webhook admission handler.
type Handler struct {
	client.Client
	config  *Config
	decoder *admission.Decoder
	logger  logr.Logger
}

// NewHandler creates a new handler for Sentinel Webhook.
func NewHandler(mgr manager.Manager, config *Config) (*Handler, error) {
	decoder, err := admission.NewDecoder(mgr.GetScheme())
	if err != nil {
		return nil, err
	}

	return &Handler{
		Client:  mgr.GetClient(),
		config:  config,
		decoder: decoder,
		logger:  log.Log.WithName(handlerName),
	}, nil
}

// Handle handles admission requests and prevents unintended changes to resources created by etcd-druid.
func (h *Handler) Handle(ctx context.Context, req admission.Request) admission.Response {
	var (
		requestGK       = schema.GroupKind{Group: req.Kind.Group, Kind: req.Kind.Kind}
		decodeOldObject bool
		obj             client.Object
		err             error
	)

	log := h.logger.WithValues("resourceGroup", req.Kind.Group, "resourceKind", req.Kind.Kind, "name", req.Name, "namespace", req.Namespace, "operation", req.Operation, "user", req.UserInfo.Username)
	log.Info("Sentinel webhook invoked")

	if req.Operation == admissionv1.Delete {
		decodeOldObject = true
	}

	switch requestGK {
	case corev1.SchemeGroupVersion.WithKind("ServiceAccount").GroupKind():
		obj, err = h.decodeServiceAccount(req, decodeOldObject)
	case corev1.SchemeGroupVersion.WithKind("Service").GroupKind():
		obj, err = h.decodeService(req, decodeOldObject)
	case corev1.SchemeGroupVersion.WithKind("ConfigMap").GroupKind():
		obj, err = h.decodeConfigMap(req, decodeOldObject)
	case rbacv1.SchemeGroupVersion.WithKind("Role").GroupKind():
		obj, err = h.decodeRole(req, decodeOldObject)
	case rbacv1.SchemeGroupVersion.WithKind("RoleBinding").GroupKind():
		obj, err = h.decodeRoleBinding(req, decodeOldObject)
	case appsv1.SchemeGroupVersion.WithKind("StatefulSet").GroupKind():
		obj, err = h.decodeStatefulSet(req, decodeOldObject)
	case policyv1.SchemeGroupVersion.WithKind("PodDisruptionBudget").GroupKind():
		obj, err = h.decodePodDisruptionBudget(req, decodeOldObject)
	case batchv1.SchemeGroupVersion.WithKind("Job").GroupKind():
		obj, err = h.decodeJob(req, decodeOldObject)
	case coordinationv1.SchemeGroupVersion.WithKind("Lease").GroupKind():
		if req.Operation == admissionv1.Update {
			return admission.Allowed("lease resource can be freely updated")
		}
		obj, err = h.decodeLease(req, decodeOldObject)
	default:
		return admission.Allowed(fmt.Sprintf("unexpected resource type: %s", requestGK))
	}
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	etcdName, hasLabel := obj.GetLabels()[druidv1alpha1.LabelPartOfKey]
	if !hasLabel {
		return admission.Allowed(fmt.Sprintf("label %s not found on resource", druidv1alpha1.LabelPartOfKey))
	}

	etcd := &druidv1alpha1.Etcd{}
	if err := h.Get(ctx, types.NamespacedName{Name: etcdName, Namespace: req.Namespace}, etcd); err != nil {
		if apierrors.IsNotFound(err) {
			return admission.Allowed(fmt.Sprintf("corresponding etcd %s not found", etcdName))
		}
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// allow changes to resources if etcd spec reconciliation is currently suspended
	if etcd.IsReconciliationSuspended() {
		return admission.Allowed(fmt.Sprintf("spec reconciliation of etcd %s is currently suspended", etcd.Name))
	}

	// allow changes to resources if etcd has annotation druid.gardener.cloud/resource-protection: false
	if !etcd.AreManagedResourcesProtected() {
		return admission.Allowed(fmt.Sprintf("changes allowed, since etcd %s has annotation %s: false", etcd.Name, druidv1alpha1.ResourceProtectionAnnotation))
	}

	// allow operations on resources if any etcd operation is currently being reconciled, but only by etcd-druid,
	// and allow exempt service accounts to make changes to resources, but only if etcd is not currently being reconciled.
	if etcd.IsReconciliationInProgress() {
		if req.UserInfo.Username == h.config.ReconcilerServiceAccount {
			return admission.Allowed(fmt.Sprintf("ongoing reconciliation of etcd %s by etcd-druid requires changes to resources", etcd.Name))
		}
		return admission.Denied(fmt.Sprintf("no external intervention allowed during ongoing reconciliation of etcd %s by etcd-druid", etcd.Name))
	} else {
		for _, sa := range h.config.ExemptServiceAccounts {
			if req.UserInfo.Username == sa {
				return admission.Allowed(fmt.Sprintf("operations on etcd %s by service account %s is exempt from Sentinel Webhook checks", etcd.Name, sa))
			}
		}
	}

	return admission.Denied(fmt.Sprintf("changes disallowed, since no ongoing processing of etcd %s by etcd-druid", etcd.Name))
}

func (h *Handler) decodeServiceAccount(req admission.Request, decodeOldObject bool) (client.Object, error) {
	sa := &corev1.ServiceAccount{}
	if decodeOldObject {
		if err := h.decoder.DecodeRaw(req.OldObject, sa); err != nil {
			return nil, err
		}
		return sa, nil
	}
	if err := h.decoder.Decode(req, sa); err != nil {
		return nil, err
	}
	return sa, nil
}

func (h *Handler) decodeService(req admission.Request, decodeOldObject bool) (client.Object, error) {
	svc := &corev1.Service{}
	if decodeOldObject {
		if err := h.decoder.DecodeRaw(req.OldObject, svc); err != nil {
			return nil, err
		}
		return svc, nil
	}
	if err := h.decoder.Decode(req, svc); err != nil {
		return nil, err
	}
	return svc, nil
}

func (h *Handler) decodeConfigMap(req admission.Request, decodeOldObject bool) (client.Object, error) {
	cm := &corev1.ConfigMap{}
	if decodeOldObject {
		if err := h.decoder.DecodeRaw(req.OldObject, cm); err != nil {
			return nil, err
		}
		return cm, nil
	}
	if err := h.decoder.Decode(req, cm); err != nil {
		return nil, err
	}
	return cm, nil
}

func (h *Handler) decodeRole(req admission.Request, decodeOldObject bool) (client.Object, error) {
	role := &rbacv1.Role{}
	if decodeOldObject {
		if err := h.decoder.DecodeRaw(req.OldObject, role); err != nil {
			return nil, err
		}
		return role, nil
	}
	if err := h.decoder.Decode(req, role); err != nil {
		return nil, err
	}
	return role, nil
}

func (h *Handler) decodeRoleBinding(req admission.Request, decodeOldObject bool) (client.Object, error) {
	rb := &rbacv1.RoleBinding{}
	if decodeOldObject {
		if err := h.decoder.DecodeRaw(req.OldObject, rb); err != nil {
			return nil, err
		}
		return rb, nil
	}
	if err := h.decoder.Decode(req, rb); err != nil {
		return nil, err
	}
	return rb, nil
}

func (h *Handler) decodeStatefulSet(req admission.Request, decodeOldObject bool) (client.Object, error) {
	sts := &appsv1.StatefulSet{}
	if decodeOldObject {
		if err := h.decoder.DecodeRaw(req.OldObject, sts); err != nil {
			return nil, err
		}
		return sts, nil
	}
	if err := h.decoder.Decode(req, sts); err != nil {
		return nil, err
	}
	return sts, nil
}

func (h *Handler) decodePodDisruptionBudget(req admission.Request, decodeOldObject bool) (client.Object, error) {
	pdb := &policyv1.PodDisruptionBudget{}
	if decodeOldObject {
		if err := h.decoder.DecodeRaw(req.OldObject, pdb); err != nil {
			return nil, err
		}
		return pdb, nil
	}
	if err := h.decoder.Decode(req, pdb); err != nil {
		return nil, err
	}
	return pdb, nil
}

func (h *Handler) decodeJob(req admission.Request, decodeOldObject bool) (client.Object, error) {
	job := &batchv1.Job{}
	if decodeOldObject {
		if err := h.decoder.DecodeRaw(req.OldObject, job); err != nil {
			return nil, err
		}
		return job, nil
	}
	if err := h.decoder.Decode(req, job); err != nil {
		return nil, err
	}
	return job, nil
}

func (h *Handler) decodeLease(req admission.Request, decodeOldObject bool) (client.Object, error) {
	lease := &coordinationv1.Lease{}
	if decodeOldObject {
		if err := h.decoder.DecodeRaw(req.OldObject, lease); err != nil {
			return nil, err
		}
		return lease, nil
	}
	if err := h.decoder.Decode(req, lease); err != nil {
		return nil, err
	}
	return lease, nil
}
