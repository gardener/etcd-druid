//go:build !ignore_autogenerated
// +build !ignore_autogenerated

// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0
// Code generated by deepcopy-gen. DO NOT EDIT.

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BackupSpec) DeepCopyInto(out *BackupSpec) {
	*out = *in
	if in.Port != nil {
		in, out := &in.Port, &out.Port
		*out = new(int32)
		**out = **in
	}
	if in.TLS != nil {
		in, out := &in.TLS, &out.TLS
		*out = new(TLSConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.Image != nil {
		in, out := &in.Image, &out.Image
		*out = new(string)
		**out = **in
	}
	if in.Store != nil {
		in, out := &in.Store, &out.Store
		*out = new(StoreSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = new(v1.ResourceRequirements)
		(*in).DeepCopyInto(*out)
	}
	if in.CompactionResources != nil {
		in, out := &in.CompactionResources, &out.CompactionResources
		*out = new(v1.ResourceRequirements)
		(*in).DeepCopyInto(*out)
	}
	if in.FullSnapshotSchedule != nil {
		in, out := &in.FullSnapshotSchedule, &out.FullSnapshotSchedule
		*out = new(string)
		**out = **in
	}
	if in.GarbageCollectionPolicy != nil {
		in, out := &in.GarbageCollectionPolicy, &out.GarbageCollectionPolicy
		*out = new(GarbageCollectionPolicy)
		**out = **in
	}
	if in.MaxBackupsLimitBasedGC != nil {
		in, out := &in.MaxBackupsLimitBasedGC, &out.MaxBackupsLimitBasedGC
		*out = new(int32)
		**out = **in
	}
	if in.GarbageCollectionPeriod != nil {
		in, out := &in.GarbageCollectionPeriod, &out.GarbageCollectionPeriod
		*out = new(metav1.Duration)
		**out = **in
	}
	if in.DeltaSnapshotPeriod != nil {
		in, out := &in.DeltaSnapshotPeriod, &out.DeltaSnapshotPeriod
		*out = new(metav1.Duration)
		**out = **in
	}
	if in.DeltaSnapshotMemoryLimit != nil {
		in, out := &in.DeltaSnapshotMemoryLimit, &out.DeltaSnapshotMemoryLimit
		x := (*in).DeepCopy()
		*out = &x
	}
	if in.DeltaSnapshotRetentionPeriod != nil {
		in, out := &in.DeltaSnapshotRetentionPeriod, &out.DeltaSnapshotRetentionPeriod
		*out = new(metav1.Duration)
		**out = **in
	}
	if in.SnapshotCompression != nil {
		in, out := &in.SnapshotCompression, &out.SnapshotCompression
		*out = new(CompressionSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.EnableProfiling != nil {
		in, out := &in.EnableProfiling, &out.EnableProfiling
		*out = new(bool)
		**out = **in
	}
	if in.EtcdSnapshotTimeout != nil {
		in, out := &in.EtcdSnapshotTimeout, &out.EtcdSnapshotTimeout
		*out = new(metav1.Duration)
		**out = **in
	}
	if in.LeaderElection != nil {
		in, out := &in.LeaderElection, &out.LeaderElection
		*out = new(LeaderElectionSpec)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BackupSpec.
func (in *BackupSpec) DeepCopy() *BackupSpec {
	if in == nil {
		return nil
	}
	out := new(BackupSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClientService) DeepCopyInto(out *ClientService) {
	*out = *in
	if in.Annotations != nil {
		in, out := &in.Annotations, &out.Annotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Labels != nil {
		in, out := &in.Labels, &out.Labels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.TrafficDistribution != nil {
		in, out := &in.TrafficDistribution, &out.TrafficDistribution
		*out = new(string)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClientService.
func (in *ClientService) DeepCopy() *ClientService {
	if in == nil {
		return nil
	}
	out := new(ClientService)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CompressionSpec) DeepCopyInto(out *CompressionSpec) {
	*out = *in
	if in.Enabled != nil {
		in, out := &in.Enabled, &out.Enabled
		*out = new(bool)
		**out = **in
	}
	if in.Policy != nil {
		in, out := &in.Policy, &out.Policy
		*out = new(CompressionPolicy)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CompressionSpec.
func (in *CompressionSpec) DeepCopy() *CompressionSpec {
	if in == nil {
		return nil
	}
	out := new(CompressionSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Condition) DeepCopyInto(out *Condition) {
	*out = *in
	in.LastTransitionTime.DeepCopyInto(&out.LastTransitionTime)
	in.LastUpdateTime.DeepCopyInto(&out.LastUpdateTime)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Condition.
func (in *Condition) DeepCopy() *Condition {
	if in == nil {
		return nil
	}
	out := new(Condition)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CrossVersionObjectReference) DeepCopyInto(out *CrossVersionObjectReference) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CrossVersionObjectReference.
func (in *CrossVersionObjectReference) DeepCopy() *CrossVersionObjectReference {
	if in == nil {
		return nil
	}
	out := new(CrossVersionObjectReference)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Etcd) DeepCopyInto(out *Etcd) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Etcd.
func (in *Etcd) DeepCopy() *Etcd {
	if in == nil {
		return nil
	}
	out := new(Etcd)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Etcd) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EtcdConfig) DeepCopyInto(out *EtcdConfig) {
	*out = *in
	if in.Quota != nil {
		in, out := &in.Quota, &out.Quota
		x := (*in).DeepCopy()
		*out = &x
	}
	if in.SnapshotCount != nil {
		in, out := &in.SnapshotCount, &out.SnapshotCount
		*out = new(int64)
		**out = **in
	}
	if in.DefragmentationSchedule != nil {
		in, out := &in.DefragmentationSchedule, &out.DefragmentationSchedule
		*out = new(string)
		**out = **in
	}
	if in.ServerPort != nil {
		in, out := &in.ServerPort, &out.ServerPort
		*out = new(int32)
		**out = **in
	}
	if in.ClientPort != nil {
		in, out := &in.ClientPort, &out.ClientPort
		*out = new(int32)
		**out = **in
	}
	if in.WrapperPort != nil {
		in, out := &in.WrapperPort, &out.WrapperPort
		*out = new(int32)
		**out = **in
	}
	if in.Image != nil {
		in, out := &in.Image, &out.Image
		*out = new(string)
		**out = **in
	}
	if in.AuthSecretRef != nil {
		in, out := &in.AuthSecretRef, &out.AuthSecretRef
		*out = new(v1.SecretReference)
		**out = **in
	}
	if in.Metrics != nil {
		in, out := &in.Metrics, &out.Metrics
		*out = new(MetricsLevel)
		**out = **in
	}
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = new(v1.ResourceRequirements)
		(*in).DeepCopyInto(*out)
	}
	if in.ClientUrlTLS != nil {
		in, out := &in.ClientUrlTLS, &out.ClientUrlTLS
		*out = new(TLSConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.PeerUrlTLS != nil {
		in, out := &in.PeerUrlTLS, &out.PeerUrlTLS
		*out = new(TLSConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.EtcdDefragTimeout != nil {
		in, out := &in.EtcdDefragTimeout, &out.EtcdDefragTimeout
		*out = new(metav1.Duration)
		**out = **in
	}
	if in.HeartbeatDuration != nil {
		in, out := &in.HeartbeatDuration, &out.HeartbeatDuration
		*out = new(metav1.Duration)
		**out = **in
	}
	if in.ClientService != nil {
		in, out := &in.ClientService, &out.ClientService
		*out = new(ClientService)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EtcdConfig.
func (in *EtcdConfig) DeepCopy() *EtcdConfig {
	if in == nil {
		return nil
	}
	out := new(EtcdConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EtcdCopyBackupsTask) DeepCopyInto(out *EtcdCopyBackupsTask) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EtcdCopyBackupsTask.
func (in *EtcdCopyBackupsTask) DeepCopy() *EtcdCopyBackupsTask {
	if in == nil {
		return nil
	}
	out := new(EtcdCopyBackupsTask)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *EtcdCopyBackupsTask) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EtcdCopyBackupsTaskList) DeepCopyInto(out *EtcdCopyBackupsTaskList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]EtcdCopyBackupsTask, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EtcdCopyBackupsTaskList.
func (in *EtcdCopyBackupsTaskList) DeepCopy() *EtcdCopyBackupsTaskList {
	if in == nil {
		return nil
	}
	out := new(EtcdCopyBackupsTaskList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *EtcdCopyBackupsTaskList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EtcdCopyBackupsTaskSpec) DeepCopyInto(out *EtcdCopyBackupsTaskSpec) {
	*out = *in
	if in.PodLabels != nil {
		in, out := &in.PodLabels, &out.PodLabels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	in.SourceStore.DeepCopyInto(&out.SourceStore)
	in.TargetStore.DeepCopyInto(&out.TargetStore)
	if in.MaxBackupAge != nil {
		in, out := &in.MaxBackupAge, &out.MaxBackupAge
		*out = new(uint32)
		**out = **in
	}
	if in.MaxBackups != nil {
		in, out := &in.MaxBackups, &out.MaxBackups
		*out = new(uint32)
		**out = **in
	}
	if in.WaitForFinalSnapshot != nil {
		in, out := &in.WaitForFinalSnapshot, &out.WaitForFinalSnapshot
		*out = new(WaitForFinalSnapshotSpec)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EtcdCopyBackupsTaskSpec.
func (in *EtcdCopyBackupsTaskSpec) DeepCopy() *EtcdCopyBackupsTaskSpec {
	if in == nil {
		return nil
	}
	out := new(EtcdCopyBackupsTaskSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EtcdCopyBackupsTaskStatus) DeepCopyInto(out *EtcdCopyBackupsTaskStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.ObservedGeneration != nil {
		in, out := &in.ObservedGeneration, &out.ObservedGeneration
		*out = new(int64)
		**out = **in
	}
	if in.LastError != nil {
		in, out := &in.LastError, &out.LastError
		*out = new(string)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EtcdCopyBackupsTaskStatus.
func (in *EtcdCopyBackupsTaskStatus) DeepCopy() *EtcdCopyBackupsTaskStatus {
	if in == nil {
		return nil
	}
	out := new(EtcdCopyBackupsTaskStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EtcdList) DeepCopyInto(out *EtcdList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Etcd, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EtcdList.
func (in *EtcdList) DeepCopy() *EtcdList {
	if in == nil {
		return nil
	}
	out := new(EtcdList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *EtcdList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EtcdMemberStatus) DeepCopyInto(out *EtcdMemberStatus) {
	*out = *in
	if in.ID != nil {
		in, out := &in.ID, &out.ID
		*out = new(string)
		**out = **in
	}
	if in.Role != nil {
		in, out := &in.Role, &out.Role
		*out = new(EtcdRole)
		**out = **in
	}
	in.LastTransitionTime.DeepCopyInto(&out.LastTransitionTime)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EtcdMemberStatus.
func (in *EtcdMemberStatus) DeepCopy() *EtcdMemberStatus {
	if in == nil {
		return nil
	}
	out := new(EtcdMemberStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EtcdSpec) DeepCopyInto(out *EtcdSpec) {
	*out = *in
	if in.Selector != nil {
		in, out := &in.Selector, &out.Selector
		*out = new(metav1.LabelSelector)
		(*in).DeepCopyInto(*out)
	}
	if in.Labels != nil {
		in, out := &in.Labels, &out.Labels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Annotations != nil {
		in, out := &in.Annotations, &out.Annotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	in.Etcd.DeepCopyInto(&out.Etcd)
	in.Backup.DeepCopyInto(&out.Backup)
	in.Common.DeepCopyInto(&out.Common)
	in.SchedulingConstraints.DeepCopyInto(&out.SchedulingConstraints)
	if in.PriorityClassName != nil {
		in, out := &in.PriorityClassName, &out.PriorityClassName
		*out = new(string)
		**out = **in
	}
	if in.StorageClass != nil {
		in, out := &in.StorageClass, &out.StorageClass
		*out = new(string)
		**out = **in
	}
	if in.StorageCapacity != nil {
		in, out := &in.StorageCapacity, &out.StorageCapacity
		x := (*in).DeepCopy()
		*out = &x
	}
	if in.VolumeClaimTemplate != nil {
		in, out := &in.VolumeClaimTemplate, &out.VolumeClaimTemplate
		*out = new(string)
		**out = **in
	}
	if in.RunAsRoot != nil {
		in, out := &in.RunAsRoot, &out.RunAsRoot
		*out = new(bool)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EtcdSpec.
func (in *EtcdSpec) DeepCopy() *EtcdSpec {
	if in == nil {
		return nil
	}
	out := new(EtcdSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EtcdStatus) DeepCopyInto(out *EtcdStatus) {
	*out = *in
	if in.ObservedGeneration != nil {
		in, out := &in.ObservedGeneration, &out.ObservedGeneration
		*out = new(int64)
		**out = **in
	}
	if in.Etcd != nil {
		in, out := &in.Etcd, &out.Etcd
		*out = new(CrossVersionObjectReference)
		**out = **in
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.LastErrors != nil {
		in, out := &in.LastErrors, &out.LastErrors
		*out = make([]LastError, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.LastOperation != nil {
		in, out := &in.LastOperation, &out.LastOperation
		*out = new(LastOperation)
		(*in).DeepCopyInto(*out)
	}
	if in.Ready != nil {
		in, out := &in.Ready, &out.Ready
		*out = new(bool)
		**out = **in
	}
	if in.LabelSelector != nil {
		in, out := &in.LabelSelector, &out.LabelSelector
		*out = new(metav1.LabelSelector)
		(*in).DeepCopyInto(*out)
	}
	if in.Members != nil {
		in, out := &in.Members, &out.Members
		*out = make([]EtcdMemberStatus, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.PeerUrlTLSEnabled != nil {
		in, out := &in.PeerUrlTLSEnabled, &out.PeerUrlTLSEnabled
		*out = new(bool)
		**out = **in
	}
	if in.Selector != nil {
		in, out := &in.Selector, &out.Selector
		*out = new(string)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EtcdStatus.
func (in *EtcdStatus) DeepCopy() *EtcdStatus {
	if in == nil {
		return nil
	}
	out := new(EtcdStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LastError) DeepCopyInto(out *LastError) {
	*out = *in
	in.ObservedAt.DeepCopyInto(&out.ObservedAt)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LastError.
func (in *LastError) DeepCopy() *LastError {
	if in == nil {
		return nil
	}
	out := new(LastError)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LastOperation) DeepCopyInto(out *LastOperation) {
	*out = *in
	in.LastUpdateTime.DeepCopyInto(&out.LastUpdateTime)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LastOperation.
func (in *LastOperation) DeepCopy() *LastOperation {
	if in == nil {
		return nil
	}
	out := new(LastOperation)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LeaderElectionSpec) DeepCopyInto(out *LeaderElectionSpec) {
	*out = *in
	if in.ReelectionPeriod != nil {
		in, out := &in.ReelectionPeriod, &out.ReelectionPeriod
		*out = new(metav1.Duration)
		**out = **in
	}
	if in.EtcdConnectionTimeout != nil {
		in, out := &in.EtcdConnectionTimeout, &out.EtcdConnectionTimeout
		*out = new(metav1.Duration)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LeaderElectionSpec.
func (in *LeaderElectionSpec) DeepCopy() *LeaderElectionSpec {
	if in == nil {
		return nil
	}
	out := new(LeaderElectionSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SchedulingConstraints) DeepCopyInto(out *SchedulingConstraints) {
	*out = *in
	if in.Affinity != nil {
		in, out := &in.Affinity, &out.Affinity
		*out = new(v1.Affinity)
		(*in).DeepCopyInto(*out)
	}
	if in.TopologySpreadConstraints != nil {
		in, out := &in.TopologySpreadConstraints, &out.TopologySpreadConstraints
		*out = make([]v1.TopologySpreadConstraint, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SchedulingConstraints.
func (in *SchedulingConstraints) DeepCopy() *SchedulingConstraints {
	if in == nil {
		return nil
	}
	out := new(SchedulingConstraints)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SecretReference) DeepCopyInto(out *SecretReference) {
	*out = *in
	out.SecretReference = in.SecretReference
	if in.DataKey != nil {
		in, out := &in.DataKey, &out.DataKey
		*out = new(string)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SecretReference.
func (in *SecretReference) DeepCopy() *SecretReference {
	if in == nil {
		return nil
	}
	out := new(SecretReference)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SharedConfig) DeepCopyInto(out *SharedConfig) {
	*out = *in
	if in.AutoCompactionMode != nil {
		in, out := &in.AutoCompactionMode, &out.AutoCompactionMode
		*out = new(CompactionMode)
		**out = **in
	}
	if in.AutoCompactionRetention != nil {
		in, out := &in.AutoCompactionRetention, &out.AutoCompactionRetention
		*out = new(string)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SharedConfig.
func (in *SharedConfig) DeepCopy() *SharedConfig {
	if in == nil {
		return nil
	}
	out := new(SharedConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StoreSpec) DeepCopyInto(out *StoreSpec) {
	*out = *in
	if in.Container != nil {
		in, out := &in.Container, &out.Container
		*out = new(string)
		**out = **in
	}
	if in.Provider != nil {
		in, out := &in.Provider, &out.Provider
		*out = new(StorageProvider)
		**out = **in
	}
	if in.SecretRef != nil {
		in, out := &in.SecretRef, &out.SecretRef
		*out = new(v1.SecretReference)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StoreSpec.
func (in *StoreSpec) DeepCopy() *StoreSpec {
	if in == nil {
		return nil
	}
	out := new(StoreSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TLSConfig) DeepCopyInto(out *TLSConfig) {
	*out = *in
	in.TLSCASecretRef.DeepCopyInto(&out.TLSCASecretRef)
	out.ServerTLSSecretRef = in.ServerTLSSecretRef
	out.ClientTLSSecretRef = in.ClientTLSSecretRef
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TLSConfig.
func (in *TLSConfig) DeepCopy() *TLSConfig {
	if in == nil {
		return nil
	}
	out := new(TLSConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *WaitForFinalSnapshotSpec) DeepCopyInto(out *WaitForFinalSnapshotSpec) {
	*out = *in
	if in.Timeout != nil {
		in, out := &in.Timeout, &out.Timeout
		*out = new(metav1.Duration)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new WaitForFinalSnapshotSpec.
func (in *WaitForFinalSnapshotSpec) DeepCopy() *WaitForFinalSnapshotSpec {
	if in == nil {
		return nil
	}
	out := new(WaitForFinalSnapshotSpec)
	in.DeepCopyInto(out)
	return out
}
