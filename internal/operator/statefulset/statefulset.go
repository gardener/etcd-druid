package statefulset

import (
	"encoding/json"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"

	"github.com/gardener/etcd-druid/internal/operator/resource"
	"github.com/gardener/etcd-druid/internal/utils"
	druidutils "github.com/gardener/etcd-druid/pkg/utils"
	"github.com/gardener/gardener/pkg/controllerutils"
	gardenerutils "github.com/gardener/gardener/pkg/utils"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type _resource struct {
	client         client.Client
	logger         logr.Logger
	imageVector    imagevector.ImageVector
	UseEtcdWrapper bool
}

func New(client client.Client, logger logr.Logger, config resource.Config) resource.Operator {

	return &_resource{
		client:         client,
		logger:         logger,
		imageVector:    config.ImageVector,
		UseEtcdWrapper: config.UseEtcdWrapper,
	}
}

func (r _resource) GetExistingResourceNames(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	sts := &appsv1.StatefulSet{}
	if err := r.client.Get(ctx, getObjectKey(etcd), sts); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return resourceNames, err
	}
	resourceNames = append(resourceNames, sts.Name)
	return resourceNames, nil
}

func (r _resource) Sync(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	var (
		existingSts *appsv1.StatefulSet
		err         error
	)
	existingSts, err = r.getExistingStatefulSet(ctx, etcd)
	if err != nil {
		return err
	}

	if existingSts == nil {
		return r.createoOrPatch(ctx, etcd, etcd.Spec.Replicas)
	}
	// Check current TLS status of etcd members
	peerTLSEnabled, err := utils.IsPeerURLTLSEnabled(ctx, r.client, etcd.Namespace, etcd.Name, r.logger)
	if err != nil {
		return err
	}

	// Handling Peer TLS changes
	if r.isPeerTLSChangedToEnabled(peerTLSEnabled, etcd) {
		if err := r.createoOrPatch(ctx, etcd, *existingSts.Spec.Replicas); err != nil {
			return err
		}
		tlsEnabled, err := utils.IsPeerURLTLSEnabled(ctx, r.client, etcd.Namespace, etcd.Name, r.logger)
		if err != nil {
			return fmt.Errorf("error occured while checking IsPeerURLTLSEnabled error :%v", err)
		}
		if !tlsEnabled {
			return fmt.Errorf("TLS not yet enabled for etcd [name: %s, namespace: %s]", etcd.Name, etcd.Namespace)
		}

		if err := r.deleteAllStsPods(ctx, "deleting all sts pods", existingSts); err != nil {
			return err
		}
	}

	// Check and handle immutable field updates
	if existingSts.Generation > 1 && immutableFieldUpdate(existingSts, etcd) {
		if err := r.TriggerDelete(ctx, etcd); err != nil {
			return err
		}
	}

	return r.createoOrPatch(ctx, etcd, etcd.Spec.Replicas)
}

func (r _resource) TriggerDelete(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	return client.IgnoreNotFound(r.client.Delete(ctx, emptyStatefulset(getObjectKey(etcd))))
}

func (r *_resource) deleteAllStsPods(ctx resource.OperatorContext, opName string, sts *appsv1.StatefulSet) error {
	// Get all Pods belonging to the StatefulSet
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(sts.Namespace),
		client.MatchingLabels(sts.Spec.Selector.MatchLabels),
	}

	if err := r.client.List(ctx, podList, listOpts...); err != nil {
		r.logger.Error(err, "Failed to list pods for StatefulSet", "StatefulSet", sts.Name, "Namespace", sts.Namespace)
		return err
	}

	// Delete each Pod
	for _, pod := range podList.Items {
		if err := r.client.Delete(ctx, &pod); err != nil {
			r.logger.Error(err, "Failed to delete pod", "Pod", pod.Name, "Namespace", pod.Namespace)
			return err
		}
	}

	r.logger.Info("All pods in StatefulSet have been deleted", "StatefulSet", sts.Name, "Namespace", sts.Namespace)
	return nil
}

// isPeerTLSChangedToEnabled checks if the Peer TLS setting has changed to enabled
func (r *_resource) isPeerTLSChangedToEnabled(peerTLSEnabledStatusFromMembers bool, etcd *druidv1alpha1.Etcd) bool {
	return !peerTLSEnabledStatusFromMembers && etcd.Spec.Etcd.PeerUrlTLS != nil
}

func (r _resource) getExistingStatefulSet(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) (*appsv1.StatefulSet, error) {
	sts := emptyStatefulset(getObjectKey(etcd))
	if err := r.client.Get(ctx, getObjectKey(etcd), sts); err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return sts, nil
}

func (r _resource) getVolumes(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) ([]corev1.Volume, error) {
	vs := []corev1.Volume{
		{
			Name: "etcd-config-file",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: etcd.GetConfigMapName(),
					},
					Items: []corev1.KeyToPath{
						{
							Key:  "etcd.conf.yaml",
							Path: "etcd.conf.yaml",
						},
					},
					DefaultMode: pointer.Int32(0644),
				},
			},
		},
	}

	if etcd.Spec.Etcd.ClientUrlTLS != nil {
		vs = append(vs, corev1.Volume{
			Name: "client-url-ca-etcd",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.Name,
				},
			},
		},
			corev1.Volume{
				Name: "client-url-etcd-server-tls",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: etcd.Spec.Etcd.ClientUrlTLS.ServerTLSSecretRef.Name,
					},
				},
			},
			corev1.Volume{
				Name: "client-url-etcd-client-tls",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: etcd.Spec.Etcd.ClientUrlTLS.ClientTLSSecretRef.Name,
					},
				},
			})
	}

	if etcd.Spec.Etcd.PeerUrlTLS != nil {
		vs = append(vs, corev1.Volume{
			Name: "peer-url-ca-etcd",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: etcd.Spec.Etcd.PeerUrlTLS.TLSCASecretRef.Name,
				},
			},
		},
			corev1.Volume{
				Name: "peer-url-etcd-server-tls",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: etcd.Spec.Etcd.PeerUrlTLS.ServerTLSSecretRef.Name,
					},
				},
			})
	}

	if etcd.Spec.Backup.Store == nil {
		return vs, nil
	}

	storeValues := etcd.Spec.Backup.Store
	provider, err := utils.StorageProviderFromInfraProvider(storeValues.Provider)
	if err != nil {
		return vs, nil
	}

	switch provider {
	case "Local":
		hostPath, err := utils.GetHostMountPathFromSecretRef(ctx, r.client, r.logger, storeValues, etcd.GetNamespace())
		if err != nil {
			return nil, err
		}

		hpt := corev1.HostPathDirectory
		vs = append(vs, corev1.Volume{
			Name: "host-storage",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: hostPath + "/" + pointer.StringDeref(storeValues.Container, ""),
					Type: &hpt,
				},
			},
		})
	case utils.GCS, utils.S3, utils.OSS, utils.ABS, utils.Swift, utils.OCS:
		if storeValues.SecretRef == nil {
			return nil, fmt.Errorf("no secretRef configured for backup store")
		}

		vs = append(vs, corev1.Volume{
			Name: "etcd-backup",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: storeValues.SecretRef.Name,
				},
			},
		})
	}

	return vs, nil
}

func (r _resource) createoOrPatch(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd, replicas int32) error {
	desiredStatefulSet := emptyStatefulset(getObjectKey(etcd))
	etcdImage, etcdBackupImage, initContainerImage, err := druidutils.GetEtcdImages(etcd, r.imageVector, r.UseEtcdWrapper)
	if err != nil {
		return err
	}
	mutatingFn := func() error {
		var stsOriginal = desiredStatefulSet.DeepCopy()
		podVolumes, err := r.getVolumes(ctx, etcd)
		if err != nil {
			return err
		}

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      etcd.GetConfigMapName(),
				Namespace: etcd.Namespace,
			},
		}
		if err := r.client.Get(ctx, client.ObjectKeyFromObject(cm), cm); err != nil {

			return err
		}
		jsonString, err := json.Marshal(cm.Data)
		if err != nil {
			return err
		}

		configMapChecksum := gardenerutils.ComputeSHA256Hex(jsonString)

		desiredStatefulSet.ObjectMeta = extractObjectMetaFromEtcd(etcd)
		desiredStatefulSet.Spec.Replicas = &replicas
		desiredStatefulSet.Spec.ServiceName = etcd.GetPeerServiceName()
		desiredStatefulSet.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: etcd.GetDefaultLabels(),
		}

		desiredStatefulSet.Spec.PodManagementPolicy = appsv1.ParallelPodManagement
		desiredStatefulSet.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.RollingUpdateStatefulSetStrategyType,
		}
		desiredStatefulSet.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: getvolumeClaimTemplateName(etcd),
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					Resources: getStorageReq(etcd),
				},
			},
		}

		desiredStatefulSet.Spec.Template.ObjectMeta = extractPodTemplateObjectMetaFromEtcd(etcd)
		if len(configMapChecksum) > 0 {
			desiredStatefulSet.Spec.Template.Annotations = utils.MergeStringMaps(map[string]string{
				"checksum/etcd-configmap": configMapChecksum,
			}, etcd.Spec.Annotations)
		}
		desiredStatefulSet.Spec.Template.Spec = corev1.PodSpec{
			HostAliases: []corev1.HostAlias{
				{
					IP:        "127.0.0.1",
					Hostnames: []string{etcd.Name + "-local"},
				},
			},
			ServiceAccountName:        etcd.GetServiceAccountName(),
			Affinity:                  etcd.Spec.SchedulingConstraints.Affinity,
			TopologySpreadConstraints: etcd.Spec.SchedulingConstraints.TopologySpreadConstraints,
			Containers: []corev1.Container{
				{
					Name:            "etcd",
					Image:           *etcdImage,
					ImagePullPolicy: corev1.PullIfNotPresent,
					Args:            getEtcdCommandArgs(r.UseEtcdWrapper, etcd),
					ReadinessProbe: &corev1.Probe{
						ProbeHandler:        getReadinessHandler(r.UseEtcdWrapper, etcd),
						InitialDelaySeconds: 15,
						PeriodSeconds:       5,
						FailureThreshold:    5,
					},
					Ports:        getEtcdPorts(),
					Resources:    getEtcdResources(etcd),
					Env:          getEtcdEnvVars(etcd),
					VolumeMounts: getEtcdVolumeMounts(etcd),
				},
				{
					Name:            "backup-restore",
					Image:           *etcdBackupImage,
					ImagePullPolicy: corev1.PullIfNotPresent,
					Args:            getBackupRestoreCommandArgs(etcd),
					Ports:           getBackupPorts(),
					Resources:       getBackupResources(etcd),
					Env:             getBackupRestoreEnvVars(etcd),
					VolumeMounts:    getBackupRestoreVolumeMounts(r.UseEtcdWrapper, etcd),
					SecurityContext: &corev1.SecurityContext{
						Capabilities: &corev1.Capabilities{
							Add: []corev1.Capability{
								"SYS_PTRACE",
							},
						},
					},
				},
			},
			ShareProcessNamespace: pointer.Bool(true),
			Volumes:               podVolumes,
		}

		if etcd.Spec.StorageClass != nil && *etcd.Spec.StorageClass != "" {
			desiredStatefulSet.Spec.VolumeClaimTemplates[0].Spec.StorageClassName = etcd.Spec.StorageClass
		}
		if etcd.Spec.PriorityClassName != nil {
			desiredStatefulSet.Spec.Template.Spec.PriorityClassName = *etcd.Spec.PriorityClassName
		}
		if r.UseEtcdWrapper {
			// sections to add only when using etcd wrapper
			// TODO: @aaronfern add this back to desiredStatefulSet.Spec when UseEtcdWrapper becomes GA
			desiredStatefulSet.Spec.Template.Spec.InitContainers = []corev1.Container{
				{
					Name:            "change-permissions",
					Image:           *initContainerImage,
					ImagePullPolicy: corev1.PullIfNotPresent,
					Command:         []string{"sh", "-c", "--"},
					Args:            []string{"chown -R 65532:65532 /var/etcd/data"},
					VolumeMounts:    getEtcdVolumeMounts(etcd),
					SecurityContext: &corev1.SecurityContext{
						RunAsGroup:   pointer.Int64(0),
						RunAsNonRoot: pointer.Bool(false),
						RunAsUser:    pointer.Int64(0),
					},
				},
			}
			if etcd.Spec.Backup.Store != nil {
				// Special container to change permissions of backup bucket folder to 65532 (nonroot)
				// Only used with local provider
				prov, _ := utils.StorageProviderFromInfraProvider(etcd.Spec.Backup.Store.Provider)
				if prov == utils.Local {
					desiredStatefulSet.Spec.Template.Spec.InitContainers = append(desiredStatefulSet.Spec.Template.Spec.InitContainers, corev1.Container{
						Name:            "change-backup-bucket-permissions",
						Image:           *initContainerImage,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Command:         []string{"sh", "-c", "--"},
						Args:            []string{fmt.Sprintf("chown -R 65532:65532 /home/nonroot/%s", *etcd.Spec.Backup.Store.Container)},
						VolumeMounts:    getBackupRestoreVolumeMounts(r.UseEtcdWrapper, etcd),
						SecurityContext: &corev1.SecurityContext{
							RunAsGroup:   pointer.Int64(0),
							RunAsNonRoot: pointer.Bool(false),
							RunAsUser:    pointer.Int64(0),
						},
					})
				}
			}
			desiredStatefulSet.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
				RunAsGroup:   pointer.Int64(65532),
				RunAsNonRoot: pointer.Bool(true),
				RunAsUser:    pointer.Int64(65532),
			}
		}

		if stsOriginal.Generation > 0 {
			// Keep immutable fields
			desiredStatefulSet.Spec.PodManagementPolicy = stsOriginal.Spec.PodManagementPolicy
			desiredStatefulSet.Spec.ServiceName = stsOriginal.Spec.ServiceName
		}
		return nil
	}

	operationResult, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, desiredStatefulSet, mutatingFn)
	if err != nil {
		return err
	}
	r.logger.Info("createOrPatch is completed", "namespace", desiredStatefulSet.Namespace, "name", desiredStatefulSet.Name, "operation-result", operationResult)

	return nil
}

func emptyStatefulset(objectKey client.ObjectKey) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectKey.Name,
			Namespace: objectKey.Namespace,
		},
	}
}

func getObjectKey(etcd *druidv1alpha1.Etcd) client.ObjectKey {
	return client.ObjectKey{
		Name:      etcd.Name,
		Namespace: etcd.Namespace,
	}
}

func immutableFieldUpdate(sts *appsv1.StatefulSet, etcd *druidv1alpha1.Etcd) bool {
	return sts.Spec.ServiceName != etcd.GetPeerServiceName() || sts.Spec.PodManagementPolicy != appsv1.ParallelPodManagement
}
