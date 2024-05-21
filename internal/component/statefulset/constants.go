// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package statefulset

const (
	etcdConfigFileName      = "etcd.conf.yaml"
	etcdConfigFileMountPath = "/var/etcd/config/"
)

// constants for container ports
const (
	serverPortName = "server"
	clientPortName = "client"
)
