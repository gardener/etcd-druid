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
	// ETCDImageSourceTag is the ImageSource tag for etcd image.
	ETCDImageSourceTag = "etcd-test-tag"
	// ETCDWrapperImageTag is the ImageSource tag for etcd-wrapper image.
	ETCDWrapperImageTag = "etcd-wrapper-test-tag"
	// ETCDBRImageTag is the ImageSource tag for etcd-backup-restore image.
	ETCDBRImageTag = "backup-restore-test-tag"
	// ETCDBRDistrolessImageTag is the ImageSource tag for etc-backup-restore distroless image.
	ETCDBRDistrolessImageTag = "backup-restore-distroless-test-tag"
	// InitContainerTag is the ImageSource tag for the init container image.
	InitContainerTag = "init-container-test-tag"
)

const (
	// TestConfigMapCheckSum is a test check-sum for the configmap which is stored as a value against checksum/etcd-configmap annotation put on the etcd sts pods.
	TestConfigMapCheckSum = "08ee0a880e10172e337ac57fb20411980a987405a0eaf9a05b8err69ca0f0d69"
)
