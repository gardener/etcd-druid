// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package configmap

import (
	"encoding/json"
	"fmt"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/utils"

	"github.com/gardener/gardener/pkg/controllerutils"
	gardenerutils "github.com/gardener/gardener/pkg/utils"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ErrGetConfigMap indicates an error in getting the configmap resource.
	ErrGetConfigMap druidv1alpha1.ErrorCode = "ERR_GET_CONFIGMAP"
	// ErrPreSyncConfigMap indicates an error in pre-syncing the configmap resource.
	ErrPreSyncConfigMap druidv1alpha1.ErrorCode = "ERR_PRE_SYNC_CONFIGMAP"
	// ErrSyncConfigMap indicates an error in syncing the configmap resource.
	ErrSyncConfigMap druidv1alpha1.ErrorCode = "ERR_SYNC_CONFIGMAP"
	// ErrDeleteConfigMap indicates an error in deleting the configmap resource.
	ErrDeleteConfigMap druidv1alpha1.ErrorCode = "ERR_DELETE_CONFIGMAP"
)

type _resource struct {
	client client.Client
}

type resourceCreatorOrUpdaterFn func(etcd *druidv1alpha1.Etcd, cm *corev1.ConfigMap, etcdCfg etcdConfig) error

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
func (r _resource) PreSync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	logger := ctx.Logger.WithValues("operation", component.OperationPreSync, "component", component.ConfigMapKind, "name", druidv1alpha1.GetConfigMapName(etcd.ObjectMeta), "namespace", etcd.Namespace)
	// Only if TLS is enabled for peer communication, we need to update the configmap. If not then there is nothing to be done in PreSync.
	if etcd.Spec.Etcd.PeerUrlTLS == nil {
		logger.V(4).Info("Peer TLS is not enabled, nothing to be done in PreSync")
		return nil
	}
	// check if the configuration reflects the enablement of TLS for peer communication.
	existingCm := &corev1.ConfigMap{}
	if err := r.client.Get(ctx, getObjectKey(etcd.ObjectMeta), existingCm); err != nil {
		// If there is no ConfigMap then there is nothing to be done in PreSync. Creation of the ConfigMap will be done in the Sync step.
		if errors.IsNotFound(err) {
			return nil
		}
		return druiderr.WrapError(err,
			ErrGetConfigMap,
			component.OperationPreSync,
			fmt.Sprintf("Error getting ConfigMap for etcd: %v", druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)))
	}
	existingEtcdCfg, err := deserializeEtcdConfig(existingCm)
	if err != nil {
		return err
	}
	// if there are no members configured in the `initial-cluster` or if they are already TLS enabled (https), then there is nothing to be done in PreSync.
	existingEtcdClusterSize := deriveReplicasFromInitialCluster(existingEtcdCfg)
	if existingEtcdClusterSize == 0 || isPeerUrlTLSAlreadyConfigured(existingEtcdCfg) {
		logger.V(4).Info("Either the derived replicas are 0 or TLS for Peer communication has already been configured")
		return nil
	}

	ctx.Logger.Info("Enabling TLS for Peer communication while retaining replicas & updating client TLS configuration")
	updateEtcdConfigWithPeerTLS(etcd, &existingEtcdCfg)
	// When moving from etcd-druid v0.22.x to v0.23.x the mount paths for TLS artifacts have changed. To be on the safe side regenerate the TLS config for client communication as well.
	updateEtcdConfigClientTLS(etcd, &existingEtcdCfg)
	result, err := r.createOrUpdate(ctx, etcd, existingCm, existingEtcdCfg)
	if err != nil {
		return err
	}
	ctx.Logger.Info("preSynced", "component", "configmap", "name", druidv1alpha1.GetConfigMapName(etcd.ObjectMeta), "result", result)
	return nil
}

// Sync creates or updates the configmap for the given Etcd.
func (r _resource) Sync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	//cm := emptyConfigMap(getObjectKey(etcd.ObjectMeta))
	cm := initializeConfigMap(etcd)
	etcdCfg := createEtcdConfig(etcd)
	result, err := r.createOrUpdate(ctx, etcd, cm, etcdCfg)
	if err != nil {
		return err
	}
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

func updateEtcdConfigWithPeerTLS(etcd *druidv1alpha1.Etcd, etcdCfg *etcdConfig) {
	pc := createPeerConfig(etcd, deriveReplicasFromInitialCluster(*etcdCfg))
	etcdCfg.InitialCluster = pc.initialCluster
	etcdCfg.ListenPeerUrls = pc.listenPeerUrls
	etcdCfg.AdvertisePeerUrls = pc.advertisePeerUrls
	if pc.peerSecurity != nil {
		etcdCfg.PeerSecurity = *pc.peerSecurity
	}
}

func updateEtcdConfigClientTLS(etcd *druidv1alpha1.Etcd, etcdCfg *etcdConfig) {
	_, clientSecurityConfig := getSchemeAndSecurityConfig(etcd.Spec.Etcd.ClientUrlTLS, common.VolumeMountPathEtcdCA, common.VolumeMountPathEtcdServerTLS)
	if clientSecurityConfig != nil {
		etcdCfg.ClientSecurity = *clientSecurityConfig
	}
}

func deriveReplicasFromInitialCluster(etcdCfg etcdConfig) int {
	initialCluster := etcdCfg.InitialCluster
	return len(strings.Split(initialCluster, ","))
}

func initializeConfigMap(etcd *druidv1alpha1.Etcd) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      druidv1alpha1.GetConfigMapName(etcd.ObjectMeta),
			Namespace: etcd.Namespace,
			Labels:    getLabels(etcd),
			OwnerReferences: []metav1.OwnerReference{
				druidv1alpha1.GetAsOwnerReference(etcd.ObjectMeta),
			},
		},
	}
	return cm
}

func deserializeEtcdConfig(cm *corev1.ConfigMap) (etcdConfig, error) {
	cfg := etcdConfig{}
	if data, ok := cm.Data[common.EtcdConfigFileName]; ok {
		if err := yaml.Unmarshal([]byte(data), &cfg); err != nil {
			return etcdConfig{}, druiderr.WrapError(err,
				ErrPreSyncConfigMap,
				component.OperationPreSync,
				fmt.Sprintf("Error unmarshalling etcd config from configmap: %v", druidv1alpha1.GetNamespaceName(cm.ObjectMeta)))
		}
	}
	return cfg, nil
}

func getLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	cmLabels := map[string]string{
		druidv1alpha1.LabelComponentKey: common.ComponentNameConfigMap,
		druidv1alpha1.LabelAppNameKey:   druidv1alpha1.GetConfigMapName(etcd.ObjectMeta),
	}
	return utils.MergeMaps(druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta), cmLabels)
}

func (r _resource) createOrUpdate(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, cm *corev1.ConfigMap, etcdCfg etcdConfig) (*controllerutil.OperationResult, error) {
	result, err := controllerutils.GetAndCreateOrMergePatch(ctx, r.client, cm, func() error {
		cfgYaml, err := yaml.Marshal(etcdCfg)
		if err != nil {
			return err
		}
		cm.Data = map[string]string{common.EtcdConfigFileName: string(cfgYaml)}
		return nil
	})
	if err != nil {
		return nil, druiderr.WrapError(err,
			ErrSyncConfigMap,
			"Sync",
			fmt.Sprintf("Error during create or update of configmap for etcd: %v", druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)))
	}
	checkSum, err := computeCheckSum(cm)
	if err != nil {
		return nil, druiderr.WrapError(err,
			ErrSyncConfigMap,
			"Sync",
			fmt.Sprintf("Error when computing CheckSum for configmap for etcd: %v", druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)))
	}
	ctx.Data[common.CheckSumKeyConfigMap] = checkSum
	return &result, nil
}

func getObjectKey(obj metav1.ObjectMeta) client.ObjectKey {
	return client.ObjectKey{
		Name:      druidv1alpha1.GetConfigMapName(obj),
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
	return gardenerutils.ComputeSHA256Hex(jsonData), nil
}
