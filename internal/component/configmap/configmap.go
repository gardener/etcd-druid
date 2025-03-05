// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package configmap

import (
	"encoding/json"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/utils"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// ErrGetConfigMap indicates an error in getting the configmap resource.
	ErrGetConfigMap druidv1alpha1.ErrorCode = "ERR_GET_CONFIGMAP"
	// ErrSyncConfigMap indicates an error in syncing the configmap resource.
	ErrSyncConfigMap druidv1alpha1.ErrorCode = "ERR_SYNC_CONFIGMAP"
	// ErrDeleteConfigMap indicates an error in deleting the configmap resource.
	ErrDeleteConfigMap druidv1alpha1.ErrorCode = "ERR_DELETE_CONFIGMAP"
)

type _resource struct {
	client client.Client
}

// New returns a new configmap component operator.
func New(client client.Client) component.Operator {
	return &_resource{
		client: client,
	}
}

// GetExistingResourceNames returns the name of the existing configmap for the given Etcd.
func (r _resource) GetExistingResourceNames(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	objKey := getObjectKey(etcdObjMeta)
	objMeta := &metav1.PartialObjectMetadata{}
	objMeta.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	if err := r.client.Get(ctx, objKey, objMeta); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return nil, druiderr.WrapError(err,
			ErrGetConfigMap,
			component.OperationGetExistingResourceNames,
			fmt.Sprintf("Error getting ConfigMap: %v for etcd: %v", objKey, druidv1alpha1.GetNamespaceName(etcdObjMeta)))
	}
	if metav1.IsControlledBy(objMeta, &etcdObjMeta) {
		resourceNames = append(resourceNames, objMeta.Name)
	}
	return resourceNames, nil
}

// PreSync is a no-op for the configmap component.
// TODO: @anveshreddy18: Remove the functionality in the below PreSync method after 3 releases i.e in v0.31.0
// This is a temporary functionality to remove the old configmaps from the cluster that are created by the etcd-druid.
func (r _resource) PreSync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	oldConfigMap := emptyConfigMap(getOldObjectKey(etcd.ObjectMeta))
	ctx.Logger.Info("PreSync: Deleting old configmap", "name", oldConfigMap.Name)
	if err := r.client.Delete(ctx, oldConfigMap); err != nil {
		if errors.IsNotFound(err) {
			ctx.Logger.Info("No old configmap found, ConfigMap PreSync is a no-op", "name", oldConfigMap.Name)
			return nil
		}
		return druiderr.WrapError(
			err,
			ErrDeleteConfigMap,
			component.OperationPreSync,
			fmt.Sprintf("Failed to delete old configmap %s", oldConfigMap.Name),
		)
	}
	ctx.Logger.Info("deleted", "component", "configmap", "name", oldConfigMap.Name)
	return nil
}

// Sync creates or updates the configmap for the given Etcd.
func (r _resource) Sync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	cm := emptyConfigMap(getObjectKey(etcd.ObjectMeta))
	result, err := controllerutil.CreateOrPatch(ctx, r.client, cm, func() error {
		return buildResource(etcd, cm)
	})
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncConfigMap,
			component.OperationSync,
			fmt.Sprintf("Error during create or update of configmap for etcd: %v", druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)))
	}
	checkSum, err := computeCheckSum(cm)
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncConfigMap,
			component.OperationSync,
			fmt.Sprintf("Error when computing CheckSum for configmap for etcd: %v", druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)))
	}
	ctx.Data[common.CheckSumKeyConfigMap] = checkSum
	ctx.Logger.Info("synced", "component", "configmap", "name", cm.Name, "result", result)
	return nil
}

// TriggerDelete triggers the deletion of the configmap for the given Etcd.
func (r _resource) TriggerDelete(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta) error {
	objectKey := getObjectKey(etcdObjMeta)
	ctx.Logger.Info("Triggering deletion of ConfigMap", "objectKey", objectKey)
	if err := r.client.Delete(ctx, emptyConfigMap(objectKey)); err != nil {
		if errors.IsNotFound(err) {
			ctx.Logger.Info("No ConfigMap found, Deletion is a No-Op", "objectKey", objectKey)
			return nil
		}
		return druiderr.WrapError(
			err,
			ErrDeleteConfigMap,
			component.OperationTriggerDelete,
			"Failed to delete configmap",
		)
	}
	ctx.Logger.Info("deleted", "component", "configmap", "objectKey", objectKey)
	return nil
}

func buildResource(etcd *druidv1alpha1.Etcd, cm *corev1.ConfigMap) error {
	cfg := createEtcdConfig(etcd)
	cfgYaml, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	cm.Name = druidv1alpha1.GetConfigMapName(etcd.ObjectMeta)
	cm.Namespace = etcd.Namespace
	cm.Labels = getLabels(etcd)
	cm.OwnerReferences = []metav1.OwnerReference{druidv1alpha1.GetAsOwnerReference(etcd.ObjectMeta)}
	cm.Data = map[string]string{common.EtcdConfigFileName: string(cfgYaml)}

	return nil
}

func getLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	cmLabels := map[string]string{
		druidv1alpha1.LabelComponentKey: common.ComponentNameConfigMap,
		druidv1alpha1.LabelAppNameKey:   druidv1alpha1.GetConfigMapName(etcd.ObjectMeta),
	}
	return utils.MergeMaps(druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta), cmLabels)
}

func getObjectKey(obj metav1.ObjectMeta) client.ObjectKey {
	return client.ObjectKey{
		Name:      druidv1alpha1.GetConfigMapName(obj),
		Namespace: obj.Namespace,
	}
}

func getOldObjectKey(obj metav1.ObjectMeta) client.ObjectKey {
	return client.ObjectKey{
		Name:      druidv1alpha1.GetOldConfigMapName(obj),
		Namespace: obj.Namespace,
	}
}

func emptyConfigMap(objectKey client.ObjectKey) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectKey.Name,
			Namespace: objectKey.Namespace,
		},
	}
}

func computeCheckSum(cm *corev1.ConfigMap) (string, error) {
	jsonData, err := json.Marshal(cm.Data)
	if err != nil {
		return "", err
	}
	return utils.ComputeSHA256Hex(jsonData), nil
}
