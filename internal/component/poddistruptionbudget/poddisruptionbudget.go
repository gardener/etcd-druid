// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package poddistruptionbudget

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/gardener/pkg/controllerutils"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
func (r _resource) GetExistingResourceNames(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	objectKey := getObjectKey(etcd)
	objMeta := &metav1.PartialObjectMetadata{}
	objMeta.SetGroupVersionKind(policyv1.SchemeGroupVersion.WithKind("PodDisruptionBudget"))
	if err := r.client.Get(ctx, objectKey, objMeta); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return resourceNames, druiderr.WrapError(err,
			ErrGetPodDisruptionBudget,
			"GetExistingResourceNames",
			fmt.Sprintf("Error getting PDB: %v for etcd: %v", objectKey, etcd.GetNamespaceName()))
	}
	if metav1.IsControlledBy(objMeta, etcd) {
		resourceNames = append(resourceNames, objMeta.Name)
	}
	return resourceNames, nil
}

// Sync creates or updates the pod disruption budget for the given Etcd.
func (r _resource) Sync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKey := getObjectKey(etcd)
	pdb := emptyPodDisruptionBudget(objectKey)
	result, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, pdb, func() error {
		buildResource(etcd, pdb)
		return nil
	})
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncPodDisruptionBudget,
			"Sync",
			fmt.Sprintf("Error during create or update of PDB: %v for etcd: %v", objectKey, etcd.GetNamespaceName()),
		)
	}
	ctx.Logger.Info("synced", "component", "pod-disruption-budget", "objectKey", objectKey, "result", result)
	return nil
}

// TriggerDelete triggers the deletion of the pod disruption budget for the given Etcd.
func (r _resource) TriggerDelete(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	ctx.Logger.Info("Triggering deletion of PDB")
	pdbObjectKey := getObjectKey(etcd)
	if err := client.IgnoreNotFound(r.client.Delete(ctx, emptyPodDisruptionBudget(pdbObjectKey))); err != nil {
		return druiderr.WrapError(err,
			ErrDeletePodDisruptionBudget,
			"TriggerDelete",
			fmt.Sprintf("Failed to delete PDB: %v for etcd: %v", pdbObjectKey, etcd.GetNamespaceName()))
	}
	ctx.Logger.Info("deleted", "component", "pod-disruption-budget", "objectKey", pdbObjectKey)
	return nil
}

func buildResource(etcd *druidv1alpha1.Etcd, pdb *policyv1.PodDisruptionBudget) {
	pdb.Labels = getLabels(etcd)
	pdb.OwnerReferences = []metav1.OwnerReference{etcd.GetAsOwnerReference()}
	pdb.Spec.MinAvailable = &intstr.IntOrString{
		IntVal: computePDBMinAvailable(int(etcd.Spec.Replicas)),
		Type:   intstr.Int,
	}
	pdb.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: etcd.GetDefaultLabels(),
	}
}

func getLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	pdbLabels := map[string]string{
		druidv1alpha1.LabelComponentKey: common.ComponentNamePodDisruptionBudget,
		druidv1alpha1.LabelAppNameKey:   etcd.Name,
	}
	return utils.MergeMaps(etcd.GetDefaultLabels(), pdbLabels)
}

func getObjectKey(etcd *druidv1alpha1.Etcd) client.ObjectKey {
	return client.ObjectKey{Name: etcd.Name, Namespace: etcd.Namespace}
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
	return int32(etcdReplicas/2 + 1)
}
