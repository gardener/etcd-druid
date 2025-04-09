// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

const (
	TestEtcdName = "etcd-test"
	// TestNamespace is a test namespace to be used in tests.
	TestNamespace = "test-ns"
)

// Image vector constants
const (
	// TestImageRepo is a constant for image repository name
	TestImageRepo = "test-repo"
	// ETCDWrapperImageTag is the ImageSource tag for etcd-wrapper image.
	ETCDWrapperImageTag = "etcd-wrapper-test-tag"
	// ETCDBRImageTag is the ImageSource tag for etcd-backup-restore image.
	ETCDBRImageTag = "backup-restore-test-tag"
	// InitContainerTag is the ImageSource tag for the init container image.
	InitContainerTag = "init-container-test-tag"
)

// Secret names
const (
	// ClientTLSCASecretName is the name of the kubernetes Secret containing the client CA.
	ClientTLSCASecretName = "client-url-ca-etcd" // #nosec G101 - this is not a credential itself but the name of the kubernetes secret resource.
	// ClientTLSServerCertSecretName is the name of the kubernetes Secret containing the client's server certificate.
	ClientTLSServerCertSecretName = "client-url-etcd-server-tls" // #nosec G101 - this is not a credential itself but the name of the kubernetes secret resource.
	// ClientTLSClientCertSecretName is the name of the kubernetes Secret containing the client's client certificate.
	ClientTLSClientCertSecretName = "client-url-etcd-client-tls" // #nosec G101 - this is not a credential itself but the name of the kubernetes secret resource.
	// PeerTLSCASecretName is the name of the kubernetes Secret containing the peer CA.
	PeerTLSCASecretName = "peer-url-ca-etcd" // #nosec G101 - this is not a credential itself but the name of the kubernetes secret resource.
	// PeerTLSServerCertSecretName is the name of the kubernetes Secret containing the peer's server certificate.
	PeerTLSServerCertSecretName = "peer-url-etcd-server-tls" // #nosec G101 - this is not a credential itself but the name of the kubernetes secret resource.
	// BackupRestoreTLSCASecretName is the name of the kubernetes Secret containing the etcd-backup-restore CA.
	BackupRestoreTLSCASecretName = "ca-etcdbr" // #nosec G101 - this is not a credential itself but the name of the kubernetes secret resource.
	// BackupRestoreTLSServerCertSecretName is the name of the kubernetes Secret containing the etcd-backup-restore server certificate.
	BackupRestoreTLSServerCertSecretName = "etcdbr-server-tls" // #nosec G101 - this is not a credential itself but the name of the kubernetes secret resource.
	// BackupRestoreTLSClientCertSecretName is the name of the kubernetes Secret containing the etcd-backup-restore client certificate.
	BackupRestoreTLSClientCertSecretName = "etcdbr-client-tls" // #nosec G101 - this is not a credential itself but the name of the kubernetes secret resource.
	// BackupStoreSecretName is the name of the kubernetes Secret containing the backup store credentials.
	BackupStoreSecretName = "etcd-backup"
)

const (
	// TestConfigMapCheckSum is a test check-sum for the configmap which is stored as a value against checksum/etcd-configmap annotation put on the etcd sts pods.
	TestConfigMapCheckSum = "08ee0a880e10172e337ac57fb20411980a987405a0eaf9a05b8err69ca0f0d69"
)
