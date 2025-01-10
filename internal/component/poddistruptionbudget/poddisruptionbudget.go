// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package poddistruptionbudget

import (
	"fmt"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/utils"

	"github.com/gardener/gardener/pkg/controllerutils"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ErrGetPodDisruptionBudget indicates an error in getting the pod disruption budget resource.
	ErrGetPodDisruptionBudget druidv1alpha1.ErrorCode = "ERR_GET_POD_DISRUPTION_BUDGET"
	// ErrSyncPodDisruptionBudget indicates an error in syncing the pod disruption budget resource.
	ErrSyncPodDisruptionBudget druidv1alpha1.ErrorCode = "ERR_SYNC_POD_DISRUPTION_BUDGET"
	// ErrDeletePodDisruptionBudget indicates an error in deleting the pod disruption budget resource.
	ErrDeletePodDisruptionBudget druidv1alpha1.ErrorCode = "ERR_DELETE_POD_DISRUPTION_BUDGET"
)

const (
	// annotationAllowUnhealthyPodEviction is an annotation that can be set on the Etcd resource, to allow unhealthy pod eviction.
	// WARNING: this annotation may be removed at any point of time, and is NOT meant to be generally used.
	annotationAllowUnhealthyPodEviction = "resources.druid.gardener.cloud/allow-unhealthy-pod-eviction"
)

type _resource struct {
	client client.Client
}

// New returns a new pod disruption budget component operator.
func New(client client.Client) component.Operator {
	return &_resource{
		client: client,
	}
}

// GetExistingResourceNames returns the name of the existing pod disruption budget for the given Etcd.
func (r _resource) GetExistingResourceNames(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	objectKey := getObjectKey(etcdObjMeta)
	objMeta := &metav1.PartialObjectMetadata{}
	objMeta.SetGroupVersionKind(policyv1.SchemeGroupVersion.WithKind("PodDisruptionBudget"))
	if err := r.client.Get(ctx, objectKey, objMeta); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return resourceNames, druiderr.WrapError(err,
			ErrGetPodDisruptionBudget,
			component.OperationGetExistingResourceNames,
			fmt.Sprintf("Error getting PDB: %v for etcd: %v", objectKey, druidv1alpha1.GetNamespaceName(etcdObjMeta)))
	}
	if metav1.IsControlledBy(objMeta, &etcdObjMeta) {
		resourceNames = append(resourceNames, objMeta.Name)
	}
	return resourceNames, nil
}

// PreSync is a no-op for the pod disruption budget component.
func (r _resource) PreSync(_ component.OperatorContext, _ *druidv1alpha1.Etcd) error { return nil }

// Sync creates or updates the pod disruption budget for the given Etcd.
func (r _resource) Sync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKey := getObjectKey(etcd.ObjectMeta)
	pdb := emptyPodDisruptionBudget(objectKey)
	result, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, pdb, func() error {
		buildResource(etcd, pdb)
		return nil
	})
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncPodDisruptionBudget,
			component.OperationSync,
			fmt.Sprintf("Error during create or update of PDB: %v for etcd: %v", objectKey, druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)),
		)
	}
	ctx.Logger.Info("synced", "component", "pod-disruption-budget", "objectKey", objectKey, "result", result)
	return nil
}

// TriggerDelete triggers the deletion of the pod disruption budget for the given Etcd.
func (r _resource) TriggerDelete(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta) error {
	ctx.Logger.Info("Triggering deletion of PDB")
	pdbObjectKey := getObjectKey(etcdObjMeta)
	if err := client.IgnoreNotFound(r.client.Delete(ctx, emptyPodDisruptionBudget(pdbObjectKey))); err != nil {
		return druiderr.WrapError(err,
			ErrDeletePodDisruptionBudget,
			component.OperationTriggerDelete,
			fmt.Sprintf("Failed to delete PDB: %v for etcd: %v", pdbObjectKey, druidv1alpha1.GetNamespaceName(etcdObjMeta)))
	}
	ctx.Logger.Info("deleted", "component", "pod-disruption-budget", "objectKey", pdbObjectKey)
	return nil
}

func buildResource(etcd *druidv1alpha1.Etcd, pdb *policyv1.PodDisruptionBudget) {
	pdb.Labels = getLabels(etcd)
	pdb.OwnerReferences = []metav1.OwnerReference{druidv1alpha1.GetAsOwnerReference(etcd.ObjectMeta)}
	pdb.Spec.MinAvailable = &intstr.IntOrString{
		IntVal: computePDBMinAvailable(int(etcd.Spec.Replicas)),
		Type:   intstr.Int,
	}
	pdb.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta),
	}
	if metav1.HasAnnotation(etcd.ObjectMeta, annotationAllowUnhealthyPodEviction) {
		pdb.Spec.UnhealthyPodEvictionPolicy = ptr.To(policyv1.AlwaysAllow)
	} else {
		pdb.Spec.UnhealthyPodEvictionPolicy = nil
	}
}

func getLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	pdbLabels := map[string]string{
		druidv1alpha1.LabelComponentKey: common.ComponentNamePodDisruptionBudget,
		druidv1alpha1.LabelAppNameKey:   etcd.Name,
	}
	return utils.MergeMaps(druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta), pdbLabels)
}

func getObjectKey(obj metav1.ObjectMeta) client.ObjectKey {
	return client.ObjectKey{Name: druidv1alpha1.GetPodDisruptionBudgetName(obj), Namespace: obj.Namespace}
}

func emptyPodDisruptionBudget(objectKey client.ObjectKey) *policyv1.PodDisruptionBudget {
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectKey.Name,
			Namespace: objectKey.Namespace,
		},
	}
}

func computePDBMinAvailable(etcdReplicas int) int32 {
	// do not enable PDB for single node cluster
	if etcdReplicas <= 1 {
		return 0
	}
	return int32(etcdReplicas/2 + 1) // #nosec G115 -- etcdReplicas will never cross the size of int32, so conversion is safe.
}
