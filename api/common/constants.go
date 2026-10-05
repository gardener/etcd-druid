// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package common

const (
	// EtcdFinalizerName is the name of the etcd finalizer.
	EtcdFinalizerName = "druid.gardener.cloud/etcd-druid"

	// EtcdOpsTaskFinalizerName is the name of the etcdopstask finalizer.
	EtcdOpsTaskFinalizerName = "druid.gardener.cloud/etcd-ops-task"
)

// DefaultPortEtcdClient is the default port for the etcd client.
const DefaultPortEtcdClient int32 = 2379
