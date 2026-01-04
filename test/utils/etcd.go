// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"
	"time"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
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
		Duration: 60 * time.Second,
	}
	garbageCollectionPeriod = metav1.Duration{
		Duration: 43200 * time.Second,
	}
	snapshotSchedule        = "0 */24 * * *"
	defragSchedule          = "0 */24 * * *"
	container               = "default.bkp"
	storageCapacity         = resource.MustParse("25Gi")
	storageClass            = "default"
	priorityClassName       = "class_priority"
	deltaSnapShotMemLimit   = resource.MustParse("100Mi")
	autoCompactionMode      = druidv1alpha1.Periodic
	autoCompactionRetention = "2m"
	snapshotCount           = int64(10000)
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

// EtcdBuilder is a builder for Etcd resources.
type EtcdBuilder struct {
	etcd *druidv1alpha1.Etcd
}

// EtcdBuilderWithDefaults creates a new EtcdBuilder with default values set.
func EtcdBuilderWithDefaults(name, namespace string) *EtcdBuilder {
	builder := EtcdBuilder{}
	builder.etcd = getDefaultEtcd(name, namespace)
	return &builder
}

// EtcdBuilderWithoutDefaults creates a new EtcdBuilder without any default values set.
func EtcdBuilderWithoutDefaults(name, namespace string) *EtcdBuilder {
	builder := EtcdBuilder{}
	builder.etcd = getEtcdWithoutDefaults(name, namespace)
	return &builder
}

// WithReplicas sets the replicas field on the Etcd resource.
func (eb *EtcdBuilder) WithReplicas(replicas int32) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Replicas = replicas
	return eb
}

// WithEtcdContainerImage sets the etcd container image on the Etcd resource.
func (eb *EtcdBuilder) WithEtcdContainerImage(image string) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Etcd.Image = ptr.To(image)
	return eb
}

// WithBackupRestoreContainerImage sets the backup-restore container image on the Etcd resource.
func (eb *EtcdBuilder) WithBackupRestoreContainerImage(image string) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.Image = ptr.To(image)
	return eb
}

// GetClientTLSConfig returns a default TLSConfig for client communication.
func GetClientTLSConfig() *druidv1alpha1.TLSConfig {
	return &druidv1alpha1.TLSConfig{
		TLSCASecretRef: druidv1alpha1.SecretReference{
			SecretReference: corev1.SecretReference{
				Name: ClientTLSCASecretName,
			},
			DataKey: ptr.To("ca.crt"),
		},
		ClientTLSSecretRef: corev1.SecretReference{
			Name: ClientTLSClientCertSecretName,
		},
		ServerTLSSecretRef: corev1.SecretReference{
			Name: ClientTLSServerCertSecretName,
		},
	}
}

// WithClientTLS adds client TLS configuration to the Etcd resource.
func (eb *EtcdBuilder) WithClientTLS() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Etcd.ClientUrlTLS = GetClientTLSConfig()
	return eb
}

// GetPeerTLSConfig returns a default TLSConfig for peer communication.
func GetPeerTLSConfig() *druidv1alpha1.TLSConfig {
	return &druidv1alpha1.TLSConfig{
		TLSCASecretRef: druidv1alpha1.SecretReference{
			SecretReference: corev1.SecretReference{
				Name: PeerTLSCASecretName,
			},
			DataKey: ptr.To("ca.crt"),
		},
		ServerTLSSecretRef: corev1.SecretReference{
			Name: PeerTLSServerCertSecretName,
		},
	}
}

// WithPeerTLS adds peer TLS configuration to the Etcd resource.
func (eb *EtcdBuilder) WithPeerTLS() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Etcd.PeerUrlTLS = GetPeerTLSConfig()
	return eb
}

// GetBackupRestoreTLSConfig returns a default TLSConfig for backup-restore communication.
func GetBackupRestoreTLSConfig() *druidv1alpha1.TLSConfig {
	return &druidv1alpha1.TLSConfig{
		TLSCASecretRef: druidv1alpha1.SecretReference{
			SecretReference: corev1.SecretReference{
				Name: BackupRestoreTLSCASecretName,
			},
			DataKey: ptr.To("ca.crt"),
		},
		ServerTLSSecretRef: corev1.SecretReference{
			Name: BackupRestoreTLSServerCertSecretName,
		},
		ClientTLSSecretRef: corev1.SecretReference{
			Name: BackupRestoreTLSClientCertSecretName,
		},
	}
}

// WithBackupRestoreTLS adds backup-restore TLS configuration to the Etcd resource.
func (eb *EtcdBuilder) WithBackupRestoreTLS() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.TLS = GetBackupRestoreTLSConfig()
	return eb
}

// WithDeltaSnapshotPeriod sets the delta snapshot period on the Etcd resource.
func (eb *EtcdBuilder) WithDeltaSnapshotPeriod(deltaSnapshotPeriod time.Duration) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.DeltaSnapshotPeriod = &metav1.Duration{Duration: deltaSnapshotPeriod}
	return eb
}

// WithRunAsRoot sets the RunAsRoot field on the Etcd resource.
func (eb *EtcdBuilder) WithRunAsRoot(runAsRoot *bool) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.RunAsRoot = runAsRoot
	return eb
}

// WithReadyStatus sets the status of the Etcd resource to ready.
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
		Ready:           ptr.To(true),
		Members:         members,
		Conditions: []druidv1alpha1.Condition{
			{Type: druidv1alpha1.ConditionTypeAllMembersReady, Status: druidv1alpha1.ConditionTrue},
		},
		LastErrors: []druidapicommon.LastError{},
		LastOperation: &druidapicommon.LastOperation{
			Type:           druidv1alpha1.LastOperationTypeReconcile,
			State:          druidv1alpha1.LastOperationStateSucceeded,
			Description:    "Reconciliation succeeded",
			RunID:          "12345",
			LastUpdateTime: metav1.Now(),
		},
	}

	return eb
}

// WithConditionAllMembersUpdated sets the ConditionTypeAllMembersUpdated condition on the Etcd resource.
func (eb *EtcdBuilder) WithConditionAllMembersUpdated(updated bool) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}

	for i := range len(eb.etcd.Status.Conditions) {
		if eb.etcd.Status.Conditions[i].Type == druidv1alpha1.ConditionTypeAllMembersUpdated {
			eb.etcd.Status.Conditions[i].LastTransitionTime = metav1.Now()
			eb.etcd.Status.Conditions[i].LastUpdateTime = metav1.Now()

			eb.etcd.Status.Conditions[i].Status, eb.etcd.Status.Conditions[i].Reason, eb.etcd.Status.Conditions[i].Message = getConditionAllMembersUpdated(updated)
			return eb
		}
	}

	status, reason, message := getConditionAllMembersUpdated(updated)
	eb.etcd.Status.Conditions = append(eb.etcd.Status.Conditions, druidv1alpha1.Condition{
		Type:               druidv1alpha1.ConditionTypeAllMembersUpdated,
		LastTransitionTime: metav1.Now(),
		LastUpdateTime:     metav1.Now(),
		Status:             status,
		Reason:             reason,
		Message:            message,
	})

	return eb
}

// WithStatusConditions sets the status conditions on the Etcd resource.
func (eb *EtcdBuilder) WithStatusConditions(conditions []druidv1alpha1.Condition) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Status.Conditions = conditions
	return eb
}

// getConditionAllMembersUpdated returns the condition status, reason and message for the ConditionTypeAllMembersUpdated condition.
func getConditionAllMembersUpdated(updated bool) (status druidv1alpha1.ConditionStatus, reason, message string) {
	if updated {
		return druidv1alpha1.ConditionTrue, "AllMembersUpdated", "All members reflect latest desired spec"
	}
	return druidv1alpha1.ConditionFalse, "NotAllMembersUpdated", "At least one member is not updated"

}

// WithLastOperation sets the last operation on the Etcd resource.
func (eb *EtcdBuilder) WithLastOperation(operation *druidapicommon.LastOperation) *EtcdBuilder {
	eb.etcd.Status.LastOperation = operation
	return eb
}

// WithStorageProvider sets the storage provider on the Etcd resource.
func (eb *EtcdBuilder) WithStorageProvider(provider druidv1alpha1.StorageProvider, prefix string) *EtcdBuilder {
	// TODO: there is no default case right now which is not very right, returning an error in a default case makes it difficult to chain
	// This should be improved later
	switch provider {
	case "aws":
		return eb.WithProviderS3(prefix)
	case "azure":
		return eb.WithProviderABS(prefix)
	case "alicloud":
		return eb.WithProviderOSS(prefix)
	case "gcp":
		return eb.WithProviderGCS(prefix)
	case "openstack":
		return eb.WithProviderSwift(prefix)
	case "local":
		return eb.WithProviderLocal(prefix)
	case "none":
		return eb.WithoutProvider()
	default:
		return eb
	}
}

// WithProviderS3 sets the S3 storage provider on the Etcd resource.
func (eb *EtcdBuilder) WithProviderS3(prefix string) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.Store = getBackupStore(
		"aws",
		prefix,
	)
	return eb
}

// WithProviderABS sets the ABS storage provider on the Etcd resource.
func (eb *EtcdBuilder) WithProviderABS(prefix string) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.Store = getBackupStore(
		"azure",
		prefix,
	)
	return eb
}

// WithProviderGCS sets the GCS storage provider on the Etcd resource.
func (eb *EtcdBuilder) WithProviderGCS(prefix string) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.Store = getBackupStore(
		"gcp",
		prefix,
	)
	return eb
}

// WithProviderSwift sets the Swift storage provider on the Etcd resource.
func (eb *EtcdBuilder) WithProviderSwift(prefix string) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.Store = getBackupStore(
		"openstack",
		prefix,
	)
	return eb
}

// WithProviderOSS sets the OSS storage provider on the Etcd resource.
func (eb *EtcdBuilder) WithProviderOSS(prefix string) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.Store = getBackupStore(
		"alicloud",
		prefix,
	)
	return eb
}

// WithProviderLocal sets the Local storage provider on the Etcd resource.
func (eb *EtcdBuilder) WithProviderLocal(prefix string) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.Store = getBackupStore(
		"local",
		prefix,
	)
	return eb
}

// WithoutBackupSecretRef removes the secretRef from the backup store of the Etcd resource.
func (eb *EtcdBuilder) WithoutBackupSecretRef() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	if eb.etcd.Spec.Backup.Store != nil && eb.etcd.Spec.Backup.Store.SecretRef != nil {
		eb.etcd.Spec.Backup.Store.SecretRef = nil
	}
	return eb
}

// WithoutProvider removes the backup store from the Etcd resource.
func (eb *EtcdBuilder) WithoutProvider() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.Store = nil
	return eb
}

// WithEtcdClientPort sets the client port on the Etcd resource.
func (eb *EtcdBuilder) WithEtcdClientPort(clientPort *int32) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Etcd.ClientPort = clientPort
	return eb
}

// WithEtcdWrapperPort sets the wrapper port on the Etcd resource.
func (eb *EtcdBuilder) WithEtcdWrapperPort(wrapperPort *int32) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Etcd.WrapperPort = wrapperPort
	return eb
}

// WithEtcdClientServiceLabels sets the labels on the Etcd client service.
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

// WithEtcdClientServiceAnnotations sets the annotations on the Etcd client service.
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

// WithEtcdClientServiceTrafficDistribution sets the traffic distribution on the Etcd client service.
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

// WithEtcdServerPort sets the server port on the Etcd resource.
func (eb *EtcdBuilder) WithEtcdServerPort(serverPort *int32) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Etcd.ServerPort = serverPort
	return eb
}

// WithBackupPort sets the backup port on the Etcd resource.
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

// WithComponentProtectionDisabled adds the annotation to disable etcd component protection on the Etcd resource.
func (eb *EtcdBuilder) WithComponentProtectionDisabled() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	return eb.WithAnnotations(map[string]string{
		druidv1alpha1.DisableEtcdComponentProtectionAnnotation: "",
	})
}

// WithSpecLabels sets the spec.labels field on the Etcd resource.
func (eb *EtcdBuilder) WithSpecLabels(labels map[string]string) *EtcdBuilder {
	eb.etcd.Spec.Labels = labels
	return eb
}

// WithGRPCGatewayEnabled enables the gRPC gateway on the Etcd resource.
func (eb *EtcdBuilder) WithGRPCGatewayEnabled() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Etcd.EnableGRPCGateway = ptr.To(true)
	return eb
}

// WithDefaultBackup creates a default backup spec and initializes etcd with it.
func (eb *EtcdBuilder) WithDefaultBackup() *EtcdBuilder {
	eb.etcd.Spec.Backup = getBackupSpec()
	return eb
}

// WithDefragmentation sets the defragmentation schedule and timeout on the Etcd resource.
func (eb *EtcdBuilder) WithDefragmentation(schedule string, timeout time.Duration) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Etcd.DefragmentationSchedule = &schedule
	eb.etcd.Spec.Etcd.EtcdDefragTimeout = &metav1.Duration{Duration: timeout}
	return eb
}

// WithGarbageCollection sets the garbage collection period, policy and max backups limit based GC on the Etcd resource.
func (eb *EtcdBuilder) WithGarbageCollection(period time.Duration, policy druidv1alpha1.GarbageCollectionPolicy, maxBackups *int32) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.GarbageCollectionPeriod = &metav1.Duration{Duration: period}
	eb.etcd.Spec.Backup.GarbageCollectionPolicy = &policy
	if maxBackups != nil && policy == druidv1alpha1.GarbageCollectionPolicyLimitBased {
		eb.etcd.Spec.Backup.MaxBackupsLimitBasedGC = maxBackups
	}
	return eb
}

// Build returns the built Etcd resource.
func (eb *EtcdBuilder) Build() *druidv1alpha1.Etcd {
	return eb.etcd
}

// getEtcdWithoutDefaults returns an Etcd resource without any default values set.
func getEtcdWithoutDefaults(name, namespace string) *druidv1alpha1.Etcd {
	return &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: druidv1alpha1.EtcdSpec{
			Labels: map[string]string{
				"app":      "etcd-statefulset",
				"instance": name,
				"name":     "etcd",
			},
			Replicas: 1,
			Backup:   druidv1alpha1.BackupSpec{},
			Etcd:     druidv1alpha1.EtcdConfig{},
			Common:   druidv1alpha1.SharedConfig{},
		},
	}
}

// getDefaultEtcd returns an Etcd resource with default values set.
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
				DefragmentationSchedule: &defragSchedule,
				EtcdDefragTimeout:       &etcdDefragTimeout,
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						"cpu":    ParseQuantity("500m"),
						"memory": ParseQuantity("1000Mi"),
					},
				},
				ClientPort:  ptr.To(common.DefaultPortEtcdClient),
				ServerPort:  ptr.To(common.DefaultPortEtcdPeer),
				WrapperPort: ptr.To(common.DefaultPortEtcdWrapper),
			},
			Common: druidv1alpha1.SharedConfig{
				AutoCompactionMode:      &autoCompactionMode,
				AutoCompactionRetention: &autoCompactionRetention,
			},
		},
	}
}

// getBackupSpec returns a BackupSpec with default values set.
func getBackupSpec() druidv1alpha1.BackupSpec {
	return druidv1alpha1.BackupSpec{
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
				Name: BackupStoreSecretName,
			},
			Container: &container,
			Provider:  &localProvider,
			Prefix:    prefix,
		},
	}
}

// getBackupStore returns a StoreSpec for the given provider and prefix.
func getBackupStore(provider druidv1alpha1.StorageProvider, prefix string) *druidv1alpha1.StoreSpec {
	return &druidv1alpha1.StoreSpec{
		Container: &container,
		Prefix:    prefix,
		Provider:  &provider,
		SecretRef: &corev1.SecretReference{
			Name: BackupStoreSecretName,
		},
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
