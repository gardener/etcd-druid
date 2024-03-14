package statefulset

// constants for volume names
const (
	// clientCAVolumeName is the name of the volume that contains the CA certificate bundle and CA certificate key used to sign certificates for client communication.
	clientCAVolumeName = "ca"
	// serverTLSVolumeName is the name of the volume that contains the server certificate used to set up the server (etcd server, etcd-backup-restore HTTP server and etcd-wrapper HTTP server).
	serverTLSVolumeName = "server-tls"
	// clientTLSVolumeName is the name of the volume that contains the client certificate used by the client to communicate to the server(etcd server, etcd-backup-restore HTTP server and etcd-wrapper HTTP server).
	clientTLSVolumeName            = "client-tls"
	peerCAVolumeName               = "peer-ca"
	peerServerTLSVolumeName        = "peer-server-tls"
	backRestoreCAVolumeName        = "back-restore-ca"
	backRestoreServerTLSVolumeName = "back-restore-server-tls"
	backRestoreClientTLSVolumeName = "back-restore-client-tls"
	etcdConfigVolumeName           = "etcd-config-file"
)

// constants for volume mount paths
const (
	backupRestoreCAVolumeMountPath        = "/var/etcdbr/ssl/ca"
	backupRestoreServerTLSVolumeMountPath = "/var/etcdbr/ssl/server"
	backupRestoreClientTLSVolumeMountPath = "/var/etcdbr/ssl/client"
)

const (
	etcdConfigFileName      = "etcd.conf.yaml"
	etcdConfigFileMountPath = "/var/etcd/config/"
)

const (
	// etcdDataVolumeMountPath is the path on etcd and etcd-backup-restore containers where etcd data directory is hosted.
	etcdDataVolumeMountPath = "/var/etcd/data"
)
