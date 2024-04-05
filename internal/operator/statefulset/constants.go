// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package statefulset

// constants for volume names
const (
	// etcdCAVolumeName is the name of the volume that contains the CA certificate bundle and CA certificate key used to sign certificates for client communication.
	etcdCAVolumeName = "etcd-ca"
	// etcdServerTLSVolumeName is the name of the volume that contains the server certificate used to set up the server (etcd server, etcd-backup-restore HTTP server and etcd-wrapper HTTP server).
	etcdServerTLSVolumeName = "etcd-server-tls"
	// etcdClientTLSVolumeName is the name of the volume that contains the client certificate used by the client to communicate to the server(etcd server, etcd-backup-restore HTTP server and etcd-wrapper HTTP server).
	etcdClientTLSVolumeName          = "etcd-client-tls"
	etcdPeerCAVolumeName             = "etcd-peer-ca"
	etcdPeerServerTLSVolumeName      = "etcd-peer-server-tls"
	backupRestoreCAVolumeName        = "backup-restore-ca"
	backupRestoreServerTLSVolumeName = "backup-restore-server-tls"
	backupRestoreClientTLSVolumeName = "backup-restore-client-tls"
	etcdConfigVolumeName             = "etcd-config-file"
	localBackupVolumeName            = "local-backup"
	providerBackupVolumeName         = "etcd-backup"
)

// constants for volume mount paths
const (
	EtcdCAVolumeMountPath                 = "/var/etcd/ssl/ca"
	EtcdServerTLSVolumeMountPath          = "/var/etcd/ssl/server"
	etcdClientTLSVolumeMountPath          = "/var/etcd/ssl/client"
	EtcdPeerCAVolumeMountPath             = "/var/etcd/ssl/peer/ca"
	EtcdPeerServerTLSVolumeMountPath      = "/var/etcd/ssl/peer/server"
	backupRestoreCAVolumeMountPath        = "/var/etcdbr/ssl/ca"
	backupRestoreServerTLSVolumeMountPath = "/var/etcdbr/ssl/server"
	backupRestoreClientTLSVolumeMountPath = "/var/etcdbr/ssl/client"
	GCSBackupVolumeMountPath              = "/var/.gcp/"
	NonGCSProviderBackupVolumeMountPath   = "/var/etcd-backup/"
)

const (
	etcdConfigFileName      = "etcd.conf.yaml"
	etcdConfigFileMountPath = "/var/etcd/config/"
)

const (
	// EtcdDataVolumeMountPath is the path on etcd and etcd-backup-restore containers where etcd data directory is hosted.
	EtcdDataVolumeMountPath = "/var/etcd/data"
)

// constants for container ports
const (
	serverPortName = "server"
	clientPortName = "client"
)
