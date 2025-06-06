//go:build !ignore_autogenerated
// +build !ignore_autogenerated

// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0
// Code generated by deepcopy-gen. DO NOT EDIT.

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClientConnectionConfiguration) DeepCopyInto(out *ClientConnectionConfiguration) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClientConnectionConfiguration.
func (in *ClientConnectionConfiguration) DeepCopy() *ClientConnectionConfiguration {
	if in == nil {
		return nil
	}
	out := new(ClientConnectionConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CompactionControllerConfiguration) DeepCopyInto(out *CompactionControllerConfiguration) {
	*out = *in
	if in.ConcurrentSyncs != nil {
		in, out := &in.ConcurrentSyncs, &out.ConcurrentSyncs
		*out = new(int)
		**out = **in
	}
	out.ActiveDeadlineDuration = in.ActiveDeadlineDuration
	out.MetricsScrapeWaitDuration = in.MetricsScrapeWaitDuration
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CompactionControllerConfiguration.
func (in *CompactionControllerConfiguration) DeepCopy() *CompactionControllerConfiguration {
	if in == nil {
		return nil
	}
	out := new(CompactionControllerConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ControllerConfiguration) DeepCopyInto(out *ControllerConfiguration) {
	*out = *in
	in.Etcd.DeepCopyInto(&out.Etcd)
	in.Compaction.DeepCopyInto(&out.Compaction)
	in.EtcdCopyBackupsTask.DeepCopyInto(&out.EtcdCopyBackupsTask)
	in.Secret.DeepCopyInto(&out.Secret)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ControllerConfiguration.
func (in *ControllerConfiguration) DeepCopy() *ControllerConfiguration {
	if in == nil {
		return nil
	}
	out := new(ControllerConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EtcdComponentProtectionWebhookConfiguration) DeepCopyInto(out *EtcdComponentProtectionWebhookConfiguration) {
	*out = *in
	if in.ReconcilerServiceAccountFQDN != nil {
		in, out := &in.ReconcilerServiceAccountFQDN, &out.ReconcilerServiceAccountFQDN
		*out = new(string)
		**out = **in
	}
	if in.ServiceAccountInfo != nil {
		in, out := &in.ServiceAccountInfo, &out.ServiceAccountInfo
		*out = new(ServiceAccountInfo)
		**out = **in
	}
	if in.ExemptServiceAccounts != nil {
		in, out := &in.ExemptServiceAccounts, &out.ExemptServiceAccounts
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EtcdComponentProtectionWebhookConfiguration.
func (in *EtcdComponentProtectionWebhookConfiguration) DeepCopy() *EtcdComponentProtectionWebhookConfiguration {
	if in == nil {
		return nil
	}
	out := new(EtcdComponentProtectionWebhookConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EtcdControllerConfiguration) DeepCopyInto(out *EtcdControllerConfiguration) {
	*out = *in
	if in.ConcurrentSyncs != nil {
		in, out := &in.ConcurrentSyncs, &out.ConcurrentSyncs
		*out = new(int)
		**out = **in
	}
	out.EtcdStatusSyncPeriod = in.EtcdStatusSyncPeriod
	out.EtcdMember = in.EtcdMember
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EtcdControllerConfiguration.
func (in *EtcdControllerConfiguration) DeepCopy() *EtcdControllerConfiguration {
	if in == nil {
		return nil
	}
	out := new(EtcdControllerConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EtcdCopyBackupsTaskControllerConfiguration) DeepCopyInto(out *EtcdCopyBackupsTaskControllerConfiguration) {
	*out = *in
	if in.ConcurrentSyncs != nil {
		in, out := &in.ConcurrentSyncs, &out.ConcurrentSyncs
		*out = new(int)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EtcdCopyBackupsTaskControllerConfiguration.
func (in *EtcdCopyBackupsTaskControllerConfiguration) DeepCopy() *EtcdCopyBackupsTaskControllerConfiguration {
	if in == nil {
		return nil
	}
	out := new(EtcdCopyBackupsTaskControllerConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EtcdMemberConfiguration) DeepCopyInto(out *EtcdMemberConfiguration) {
	*out = *in
	out.NotReadyThreshold = in.NotReadyThreshold
	out.UnknownThreshold = in.UnknownThreshold
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EtcdMemberConfiguration.
func (in *EtcdMemberConfiguration) DeepCopy() *EtcdMemberConfiguration {
	if in == nil {
		return nil
	}
	out := new(EtcdMemberConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LeaderElectionConfiguration) DeepCopyInto(out *LeaderElectionConfiguration) {
	*out = *in
	out.LeaseDuration = in.LeaseDuration
	out.RenewDeadline = in.RenewDeadline
	out.RetryPeriod = in.RetryPeriod
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LeaderElectionConfiguration.
func (in *LeaderElectionConfiguration) DeepCopy() *LeaderElectionConfiguration {
	if in == nil {
		return nil
	}
	out := new(LeaderElectionConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LogConfiguration) DeepCopyInto(out *LogConfiguration) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LogConfiguration.
func (in *LogConfiguration) DeepCopy() *LogConfiguration {
	if in == nil {
		return nil
	}
	out := new(LogConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OperatorConfiguration) DeepCopyInto(out *OperatorConfiguration) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	out.ClientConnection = in.ClientConnection
	out.LeaderElection = in.LeaderElection
	in.Server.DeepCopyInto(&out.Server)
	in.Controllers.DeepCopyInto(&out.Controllers)
	in.Webhooks.DeepCopyInto(&out.Webhooks)
	if in.FeatureGates != nil {
		in, out := &in.FeatureGates, &out.FeatureGates
		*out = make(map[string]bool, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	out.Logging = in.Logging
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OperatorConfiguration.
func (in *OperatorConfiguration) DeepCopy() *OperatorConfiguration {
	if in == nil {
		return nil
	}
	out := new(OperatorConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *OperatorConfiguration) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SecretControllerConfiguration) DeepCopyInto(out *SecretControllerConfiguration) {
	*out = *in
	if in.ConcurrentSyncs != nil {
		in, out := &in.ConcurrentSyncs, &out.ConcurrentSyncs
		*out = new(int)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SecretControllerConfiguration.
func (in *SecretControllerConfiguration) DeepCopy() *SecretControllerConfiguration {
	if in == nil {
		return nil
	}
	out := new(SecretControllerConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Server) DeepCopyInto(out *Server) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Server.
func (in *Server) DeepCopy() *Server {
	if in == nil {
		return nil
	}
	out := new(Server)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ServerConfiguration) DeepCopyInto(out *ServerConfiguration) {
	*out = *in
	if in.Webhooks != nil {
		in, out := &in.Webhooks, &out.Webhooks
		*out = new(TLSServer)
		**out = **in
	}
	if in.Metrics != nil {
		in, out := &in.Metrics, &out.Metrics
		*out = new(Server)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServerConfiguration.
func (in *ServerConfiguration) DeepCopy() *ServerConfiguration {
	if in == nil {
		return nil
	}
	out := new(ServerConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ServiceAccountInfo) DeepCopyInto(out *ServiceAccountInfo) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServiceAccountInfo.
func (in *ServiceAccountInfo) DeepCopy() *ServiceAccountInfo {
	if in == nil {
		return nil
	}
	out := new(ServiceAccountInfo)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TLSServer) DeepCopyInto(out *TLSServer) {
	*out = *in
	out.Server = in.Server
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TLSServer.
func (in *TLSServer) DeepCopy() *TLSServer {
	if in == nil {
		return nil
	}
	out := new(TLSServer)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *WebhookConfiguration) DeepCopyInto(out *WebhookConfiguration) {
	*out = *in
	in.EtcdComponentProtection.DeepCopyInto(&out.EtcdComponentProtection)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new WebhookConfiguration.
func (in *WebhookConfiguration) DeepCopy() *WebhookConfiguration {
	if in == nil {
		return nil
	}
	out := new(WebhookConfiguration)
	in.DeepCopyInto(out)
	return out
}
