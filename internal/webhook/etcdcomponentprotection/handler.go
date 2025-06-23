// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdcomponentprotection

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/webhook/utils"

	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var allowedOperations = []admissionv1.Operation{admissionv1.Create, admissionv1.Connect}

// Handler is the Etcd Components protection Webhook admission handler.
// All resources that are provisioned by druid as part of etcd cluster provisioning are protected from
// unintended modification or deletion by this admission handler.
type Handler struct {
	client                       client.Client
	config                       druidconfigv1alpha1.EtcdComponentProtectionWebhookConfiguration
	decoder                      *utils.RequestDecoder
	logger                       logr.Logger
	reconcilerServiceAccountFQDN string
}

// NewHandler creates a new handler for Etcd Components Webhook.
func NewHandler(mgr manager.Manager, config druidconfigv1alpha1.EtcdComponentProtectionWebhookConfiguration) (*Handler, error) {
	reconcilerServiceAccountFQDN, err := utils.GetReconcilerServiceAccountFQDN(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create etcdcomponentprotection handler due to error in getting reconciler service account FQDN: %w", err)
	}
	return &Handler{
		client:                       mgr.GetClient(),
		config:                       config,
		decoder:                      utils.NewRequestDecoder(mgr),
		logger:                       mgr.GetLogger().WithName(handlerName),
		reconcilerServiceAccountFQDN: reconcilerServiceAccountFQDN,
	}, nil
}

// Handle handles admission requests and prevents unintended changes to resources created by etcd-druid.
func (h *Handler) Handle(ctx context.Context, req admission.Request) admission.Response {
	requestGK := utils.GetGroupKindFromRequest(req)
	log := h.logger.WithValues("name", req.Name, "namespace", req.Namespace, "resourceGroupKind", requestGK, "operation", req.Operation, "user", req.UserInfo.Username)
	log.V(1).Info("EtcdComponents webhook invoked")

	if ok, response := h.skipValidationForOperations(req.Operation); ok {
		return *response
	}

	partialObjMeta, err := h.decoder.DecodeRequestObjectAsPartialObjectMetadata(ctx, req)
	if err != nil {
		return admission.Errored(utils.DetermineStatusCode(err), err)
	}
	if partialObjMeta == nil {
		return admission.Allowed(fmt.Sprintf("resource: %v is not supported by EtcdComponents webhook", requestGK))
	}

	if !isObjManagedByDruid(partialObjMeta.ObjectMeta) {
		return admission.Allowed(fmt.Sprintf("resource: %v is not managed by druid, skipping validations", utils.CreateObjectKey(partialObjMeta)))
	}

	etcd, warnings, err := h.getParentEtcdObj(ctx, partialObjMeta)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if etcd == nil {
		return admission.Allowed(fmt.Sprintf("resource: %v  is not part of any Etcd, skipping validations", utils.CreateObjectKey(partialObjMeta))).WithWarnings(warnings...)
	}

	// allow changes to resources if Etcd has annotation druid.gardener.cloud/disable-etcd-component-protection is set.
	if !druidv1alpha1.AreManagedResourcesProtected(etcd.ObjectMeta) {
		return admission.Allowed(fmt.Sprintf("changes allowed, since Etcd %s has annotation %s", etcd.Name, druidv1alpha1.DisableEtcdComponentProtectionAnnotation))
	}

	if isRuntimeComponent(requestGK) {
		if !druidv1alpha1.IsEtcdRuntimeComponentCreationEnabled(etcd.ObjectMeta) {
			return admission.Allowed(fmt.Sprintf("Etcd %s has runtime component creation disabled, skipping validations for resource %v", etcd.Name, utils.CreateObjectKey(partialObjMeta)))
		}
	}

	// allow deletion operation on resources if the Etcd is currently being deleted, but only by etcd-druid and exempt service accounts.
	if req.Operation == admissionv1.Delete && etcd.IsDeletionInProgress() {
		return h.handleDelete(req, etcd)
	}

	return h.handleUpdate(req, etcd, partialObjMeta.ObjectMeta)
}

func (h *Handler) handleUpdate(req admission.Request, etcd *druidv1alpha1.Etcd, objMeta metav1.ObjectMeta) admission.Response {
	if req.UserInfo.Username == h.reconcilerServiceAccountFQDN {
		return admission.Allowed("update of managed resources by etcd-druid is allowed")
	}

	requestGK := utils.GetGroupKindFromRequest(req)

	// Leases (member and snapshot) will be periodically updated by etcd members.
	// Allow updates to such leases, but only by etcd members, which would use the serviceaccount deployed by druid for them.
	if requestGK == coordinationv1.SchemeGroupVersion.WithKind("Lease").GroupKind() {
		if utils.ServiceAccountMatchesUsername(etcd.GetNamespace(), druidv1alpha1.GetServiceAccountName(etcd.ObjectMeta), req.UserInfo.Username) {
			return admission.Allowed("lease resource can be freely updated by etcd members")
		}
	}

	// allow exempt service accounts to make changes to resources, but only if the Etcd is not currently being reconciled,
	// or if it is currently being reconciled and the resource is already marked for deletion, such as orphan-deleted statefulsets.
	if isServiceAccountExempted(req.UserInfo.Username, h.config.ExemptServiceAccounts) {
		if !etcd.IsReconciliationInProgress() {
			return admission.Allowed(fmt.Sprintf("operations on resources by exempt service account %s are allowed", req.UserInfo.Username))
		}
		if objMeta.DeletionTimestamp != nil {
			return admission.Allowed(fmt.Sprintf("deletion of resource by exempt service account %s is allowed during ongoing reconciliation of Etcd %s, since deletion timestamp is set on the resource", req.UserInfo.Username, etcd.Name))
		}
	}

	return admission.Denied(fmt.Sprintf("changes from service account %s are disallowed at the moment. Please consider disabling component protection by setting annotation `%s` on the parent Etcd resource", req.UserInfo.Username, druidv1alpha1.DisableEtcdComponentProtectionAnnotation))
}

func (h *Handler) handleDelete(req admission.Request, etcd *druidv1alpha1.Etcd) admission.Response {
	if req.UserInfo.Username == h.reconcilerServiceAccountFQDN {
		return admission.Allowed(fmt.Sprintf("deletion of resource by etcd-druid is allowed during deletion of Etcd %s", etcd.Name))
	}
	if slices.Contains(h.config.ExemptServiceAccounts, req.UserInfo.Username) {
		return admission.Allowed(fmt.Sprintf("deletion of resource by exempt SA %s is allowed during deletion of Etcd %s", req.UserInfo.Username, etcd.Name))
	}
	return admission.Denied(fmt.Sprintf("no external intervention allowed during ongoing deletion of Etcd %s by etcd-druid", etcd.Name))
}

func (h *Handler) skipValidationForOperations(reqOperation admissionv1.Operation) (bool, *admission.Response) {
	skipOperation := slices.Contains(allowedOperations, reqOperation)
	skipAllowedResponse := admission.Allowed(fmt.Sprintf("operation %s is allowed", reqOperation))
	if skipOperation {
		return true, &skipAllowedResponse
	}
	return false, nil
}

func (h *Handler) getParentEtcdObj(ctx context.Context, partialObjMeta *metav1.PartialObjectMetadata) (*druidv1alpha1.Etcd, admission.Warnings, error) {
	etcdName, hasLabel := partialObjMeta.GetLabels()[druidv1alpha1.LabelPartOfKey]
	if !hasLabel {
		return nil, admission.Warnings{fmt.Sprintf("cannot determine parent etcd resource, label %s not found on resource: %v", druidv1alpha1.LabelPartOfKey, utils.CreateObjectKey(partialObjMeta))}, nil
	}
	etcd := &druidv1alpha1.Etcd{}
	if err := h.client.Get(ctx, types.NamespacedName{Name: etcdName, Namespace: partialObjMeta.GetNamespace()}, etcd); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, admission.Warnings{fmt.Sprintf("parent Etcd %s not found for resource: %v", etcdName, utils.CreateObjectKey(partialObjMeta))}, nil
		}
		return nil, nil, err
	}
	return etcd, nil, nil
}

func isObjManagedByDruid(objMeta metav1.ObjectMeta) bool {
	managedBy, hasLabel := objMeta.GetLabels()[druidv1alpha1.LabelManagedByKey]
	return hasLabel && managedBy == druidv1alpha1.LabelManagedByValue
}

func isRuntimeComponent(requestGK schema.GroupKind) bool {
	return requestGK == corev1.SchemeGroupVersion.WithKind("ServiceAccount").GroupKind() ||
		requestGK == rbacv1.SchemeGroupVersion.WithKind("Role").GroupKind() ||
		requestGK == rbacv1.SchemeGroupVersion.WithKind("RoleBinding").GroupKind() ||
		requestGK == coordinationv1.SchemeGroupVersion.WithKind("Lease").GroupKind()
}

func isServiceAccountExempted(serviceAccount string, exemptedServiceAccounts []string) bool {
	return slices.Contains(exemptedServiceAccounts, serviceAccount)
}
