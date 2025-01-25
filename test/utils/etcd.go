// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	deltaSnapshotPeriod = metav1.Duration{
		Duration: 300 * time.Second,
	}
	garbageCollectionPeriod = metav1.Duration{
		Duration: 43200 * time.Second,
	}
	imageEtcd               = "eu.gcr.io/gardener-project/gardener/etcd-wrapper:v0.1.0"
	imageBR                 = "eu.gcr.io/gardener-project/gardener/etcdbrctl:v0.25.0"
	snapshotSchedule        = "0 */24 * * *"
	defragSchedule          = "0 */24 * * *"
	container               = "default.bkp"
	storageCapacity         = resource.MustParse("5Gi")
	storageClass            = "default"
	priorityClassName       = "class_priority"
	deltaSnapShotMemLimit   = resource.MustParse("100Mi")
	autoCompactionMode      = druidv1alpha1.Periodic
	autoCompactionRetention = "2m"
	snapshotCount           = int64(75000)
	quota                   = resource.MustParse("8Gi")
	localProvider           = druidv1alpha1.StorageProvider("Local")
	prefix                  = "/tmp"
	volumeClaimTemplateName = "etcd-main"
	garbageCollectionPolicy = druidv1alpha1.GarbageCollectionPolicy(druidv1alpha1.GarbageCollectionPolicyExponential)
	metricsBasic            = druidv1alpha1.Basic
	etcdSnapshotTimeout     = metav1.Duration{
		Duration: 10 * time.Minute,
	}
	etcdDefragTimeout = metav1.Duration{
		Duration: 10 * time.Minute,
	}
)

type EtcdBuilder struct {
	etcd *druidv1alpha1.Etcd
}

func EtcdBuilderWithDefaults(name, namespace string) *EtcdBuilder {
	builder := EtcdBuilder{}
	builder.etcd = getDefaultEtcd(name, namespace)
	return &builder
}

func EtcdBuilderWithoutDefaults(name, namespace string) *EtcdBuilder {
	builder := EtcdBuilder{}
	builder.etcd = getEtcdWithoutDefaults(name, namespace)
	return &builder
}

func (eb *EtcdBuilder) WithReplicas(replicas int32) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Replicas = replicas
	return eb
}

func (eb *EtcdBuilder) WithClientTLS() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	clientTLSConfig := &druidv1alpha1.TLSConfig{
		TLSCASecretRef: druidv1alpha1.SecretReference{
			SecretReference: corev1.SecretReference{
				Name: "client-url-ca-etcd",
			},
			DataKey: ptr.To("ca.crt"),
		},
		ClientTLSSecretRef: corev1.SecretReference{
			Name: "client-url-etcd-client-tls",
		},
		ServerTLSSecretRef: corev1.SecretReference{
			Name: "client-url-etcd-server-tls",
		},
	}
	eb.etcd.Spec.Etcd.ClientUrlTLS = clientTLSConfig
	eb.etcd.Spec.Backup.TLS = clientTLSConfig
	return eb
}

func (eb *EtcdBuilder) WithPeerTLS() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	peerTLSConfig := &druidv1alpha1.TLSConfig{
		TLSCASecretRef: druidv1alpha1.SecretReference{
			SecretReference: corev1.SecretReference{
				Name: "peer-url-ca-etcd",
			},
			DataKey: ptr.To("ca.crt"),
		},
		ServerTLSSecretRef: corev1.SecretReference{
			Name: "peer-url-etcd-server-tls",
		},
	}
	eb.etcd.Spec.Etcd.PeerUrlTLS = peerTLSConfig
	return eb
}

func (eb *EtcdBuilder) WithReadyStatus() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}

	members := make([]druidv1alpha1.EtcdMemberStatus, 0)
	for i := 0; i < int(eb.etcd.Spec.Replicas); i++ {
		members = append(members, druidv1alpha1.EtcdMemberStatus{Status: druidv1alpha1.EtcdMemberStatusReady})
	}
	eb.etcd.Status = druidv1alpha1.EtcdStatus{
		ReadyReplicas:   eb.etcd.Spec.Replicas,
		Replicas:        eb.etcd.Spec.Replicas,
		CurrentReplicas: eb.etcd.Spec.Replicas,
		UpdatedReplicas: eb.etcd.Spec.Replicas,
		Ready:           ptr.To(true),
		Members:         members,
		Conditions: []druidv1alpha1.Condition{
			{Type: druidv1alpha1.ConditionTypeAllMembersReady, Status: druidv1alpha1.ConditionTrue},
		},
		LastErrors: []druidv1alpha1.LastError{},
		LastOperation: &druidv1alpha1.LastOperation{
			Type:           druidv1alpha1.LastOperationTypeReconcile,
			State:          druidv1alpha1.LastOperationStateSucceeded,
			Description:    "Reconciliation succeeded",
			RunID:          "12345",
			LastUpdateTime: metav1.Now(),
		},
	}

	return eb
}

func (eb *EtcdBuilder) WithLastOperation(operation *druidv1alpha1.LastOperation) *EtcdBuilder {
	eb.etcd.Status.LastOperation = operation
	return eb
}

func (eb *EtcdBuilder) WithStorageProvider(provider druidv1alpha1.StorageProvider) *EtcdBuilder {
	// TODO: there is no default case right now which is not very right, returning an error in a default case makes it difficult to chain
	// This should be improved later
	switch provider {
	case "aws":
		return eb.WithProviderS3()
	case "azure":
		return eb.WithProviderABS()
	case "alicloud":
		return eb.WithProviderOSS()
	case "gcp":
		return eb.WithProviderGCS()
	case "openstack":
		return eb.WithProviderSwift()
	case "local":
		return eb.WithProviderLocal()
	default:
		return eb
	}
}

func (eb *EtcdBuilder) WithProviderS3() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.Store = getBackupStore(
		eb.etcd.Name,
		"aws",
	)
	return eb
}

func (eb *EtcdBuilder) WithProviderABS() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.Store = getBackupStore(
		eb.etcd.Name,
		"azure",
	)
	return eb
}

func (eb *EtcdBuilder) WithProviderGCS() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.Store = getBackupStore(
		eb.etcd.Name,
		"gcp",
	)
	return eb
}

func (eb *EtcdBuilder) WithProviderSwift() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.Store = getBackupStore(
		eb.etcd.Name,
		"openstack",
	)
	return eb
}

func (eb *EtcdBuilder) WithProviderOSS() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.Store = getBackupStore(
		eb.etcd.Name,
		"alicloud",
	)
	return eb
}

func (eb *EtcdBuilder) WithProviderLocal() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.Store = getBackupStoreForLocal(
		eb.etcd.Name,
	)
	return eb
}

func (eb *EtcdBuilder) WithEtcdClientPort(clientPort *int32) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Etcd.ClientPort = clientPort
	return eb
}

func (eb *EtcdBuilder) WithEtcdClientServiceLabels(labels map[string]string) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}

	if eb.etcd.Spec.Etcd.ClientService == nil {
		eb.etcd.Spec.Etcd.ClientService = &druidv1alpha1.ClientService{}
	}

	eb.etcd.Spec.Etcd.ClientService.Labels = MergeMaps(eb.etcd.Spec.Etcd.ClientService.Labels, labels)
	return eb
}

func (eb *EtcdBuilder) WithEtcdClientServiceAnnotations(annotations map[string]string) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}

	if eb.etcd.Spec.Etcd.ClientService == nil {
		eb.etcd.Spec.Etcd.ClientService = &druidv1alpha1.ClientService{}
	}

	eb.etcd.Spec.Etcd.ClientService.Annotations = MergeMaps(eb.etcd.Spec.Etcd.ClientService.Annotations, annotations)
	return eb
}

func (eb *EtcdBuilder) WithEtcdClientServiceTrafficDistribution(trafficDistribution *string) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}

	if eb.etcd.Spec.Etcd.ClientService == nil {
		eb.etcd.Spec.Etcd.ClientService = &druidv1alpha1.ClientService{}
	}

	eb.etcd.Spec.Etcd.ClientService.TrafficDistribution = trafficDistribution
	return eb
}

func (eb *EtcdBuilder) WithEtcdServerPort(serverPort *int32) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Etcd.ServerPort = serverPort
	return eb
}

func (eb *EtcdBuilder) WithBackupPort(port *int32) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.Port = port
	return eb
}

// WithLabels merges the labels that already exists with the ones that are passed to this method. If an entry is
// already present in the existing labels then it gets overwritten with the new value present in the passed in labels.
func (eb *EtcdBuilder) WithLabels(labels map[string]string) *EtcdBuilder {
	MergeMaps(eb.etcd.Labels, labels)
	return eb
}

// WithAnnotations merges the existing annotations if any with the annotations passed.
// Any existing entry will be replaced by the one that is present in the annotations passed to this method.
func (eb *EtcdBuilder) WithAnnotations(annotations map[string]string) *EtcdBuilder {
	eb.etcd.Annotations = MergeMaps(eb.etcd.Annotations, annotations)
	return eb
}

// WithDefaultBackup creates a default backup spec and initializes etcd with it.
func (eb *EtcdBuilder) WithDefaultBackup() *EtcdBuilder {
	eb.etcd.Spec.Backup = getBackupSpec()
	return eb
}

func (eb *EtcdBuilder) Build() *druidv1alpha1.Etcd {
	return eb.etcd
}

func getEtcdWithoutDefaults(name, namespace string) *druidv1alpha1.Etcd {
	return &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: druidv1alpha1.EtcdSpec{
			Annotations: map[string]string{
				"app":      "etcd-statefulset",
				"instance": name,
				"name":     "etcd",
				"role":     "test",
			},
			Labels: map[string]string{
				"app":      "etcd-statefulset",
				"instance": name,
				"name":     "etcd",
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":      "etcd-statefulset",
					"instance": name,
					"name":     "etcd",
				},
			},
			Replicas: 1,
			Backup:   druidv1alpha1.BackupSpec{},
			Etcd:     druidv1alpha1.EtcdConfig{},
			Common:   druidv1alpha1.SharedConfig{},
		},
	}
}

func getDefaultEtcd(name, namespace string) *druidv1alpha1.Etcd {
	return &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(uuid.NewString()),
		},
		Spec: druidv1alpha1.EtcdSpec{
			Annotations: map[string]string{
				"app":      "etcd-statefulset",
				"instance": name,
				"name":     "etcd",
				"role":     "test",
			},
			Labels: map[string]string{
				druidv1alpha1.LabelManagedByKey:                  druidv1alpha1.LabelManagedByValue,
				druidv1alpha1.LabelPartOfKey:                     name,
				"networking.gardener.cloud/to-dns":               "allowed",
				"networking.gardener.cloud/to-private-networks":  "allowed",
				"networking.gardener.cloud/to-public-networks":   "allowed",
				"networking.gardener.cloud/to-runtime-apiserver": "allowed",
				"gardener.cloud/role":                            "controlplane",
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue,
					druidv1alpha1.LabelPartOfKey:    name,
				},
			},
			Replicas:            1,
			StorageCapacity:     &storageCapacity,
			StorageClass:        &storageClass,
			PriorityClassName:   &priorityClassName,
			VolumeClaimTemplate: &volumeClaimTemplateName,
			Backup:              getBackupSpec(),
			Etcd: druidv1alpha1.EtcdConfig{
				Quota:                   &quota,
				SnapshotCount:           &snapshotCount,
				Metrics:                 &metricsBasic,
				Image:                   &imageEtcd,
				DefragmentationSchedule: &defragSchedule,
				EtcdDefragTimeout:       &etcdDefragTimeout,
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						"cpu":    ParseQuantity("500m"),
						"memory": ParseQuantity("1000Mi"),
					},
				},
				ClientPort: ptr.To(common.DefaultPortEtcdClient),
				ServerPort: ptr.To(common.DefaultPortEtcdPeer),
			},
			Common: druidv1alpha1.SharedConfig{
				AutoCompactionMode:      &autoCompactionMode,
				AutoCompactionRetention: &autoCompactionRetention,
			},
		},
	}
}

func getBackupSpec() druidv1alpha1.BackupSpec {
	return druidv1alpha1.BackupSpec{
		Image:                    &imageBR,
		Port:                     ptr.To(common.DefaultPortEtcdBackupRestore),
		FullSnapshotSchedule:     &snapshotSchedule,
		GarbageCollectionPolicy:  &garbageCollectionPolicy,
		GarbageCollectionPeriod:  &garbageCollectionPeriod,
		DeltaSnapshotPeriod:      &deltaSnapshotPeriod,
		DeltaSnapshotMemoryLimit: &deltaSnapShotMemLimit,
		EtcdSnapshotTimeout:      &etcdSnapshotTimeout,

		Resources: &corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				"cpu":    ParseQuantity("500m"),
				"memory": ParseQuantity("2Gi"),
			},
			Requests: corev1.ResourceList{
				"cpu":    ParseQuantity("23m"),
				"memory": ParseQuantity("128Mi"),
			},
		},
		Store: &druidv1alpha1.StoreSpec{
			SecretRef: &corev1.SecretReference{
				Name: "etcd-backup",
			},
			Container: &container,
			Provider:  &localProvider,
			Prefix:    prefix,
		},
	}
}

func getBackupStore(name string, provider druidv1alpha1.StorageProvider) *druidv1alpha1.StoreSpec {
	return &druidv1alpha1.StoreSpec{
		Container: &container,
		Prefix:    name,
		Provider:  &provider,
		SecretRef: &corev1.SecretReference{
			Name: "etcd-backup",
		},
	}
}

func getBackupStoreForLocal(name string) *druidv1alpha1.StoreSpec {
	provider := druidv1alpha1.StorageProvider("local")
	return &druidv1alpha1.StoreSpec{
		Container: &container,
		Prefix:    name,
		Provider:  &provider,
	}
}

// CheckEtcdOwnerReference checks if one of the owner references points to etcd owner UID.
func CheckEtcdOwnerReference(refs []metav1.OwnerReference, etcdOwnerUID types.UID) bool {
	for _, ownerRef := range refs {
		if ownerRef.UID == etcdOwnerUID {
			return true
		}
	}
	return false
}

// IsEtcdRemoved checks if the given etcd object is removed
func IsEtcdRemoved(c client.Client, name, namespace string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	etcd := &druidv1alpha1.Etcd{}
	req := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
	if err := c.Get(ctx, req, etcd); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers
			return nil
		}
		return err
	}
	return fmt.Errorf("etcd not deleted")
}
