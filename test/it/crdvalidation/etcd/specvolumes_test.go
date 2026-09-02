// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Testing validations of etcd.spec.volumes and cross-field volume-mount references.

package etcd

import (
	"testing"

	"github.com/gardener/etcd-druid/test/utils"

	corev1 "k8s.io/api/core/v1"
)

// TestValidateSpecVolumes validates that spec.volumes entries must be unique by name.
func TestValidateSpecVolumes(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
	tests := []struct {
		name      string
		etcdName  string
		volumes   []corev1.Volume
		expectErr bool
	}{
		{
			name:     "Valid: unique volume names",
			etcdName: "etcd-vol-unique-valid",
			volumes: []corev1.Volume{
				{Name: "vol-a", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
				{Name: "vol-b", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
			expectErr: false,
		},
		{
			name:     "Invalid: duplicate volume name",
			etcdName: "etcd-vol-unique-invalid",
			volumes: []corev1.Volume{
				{Name: "vol-a", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
				{Name: "vol-a", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
			expectErr: true,
		},
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(1).Build()
			etcd.Spec.Volumes = test.volumes
			validateEtcdCreation(g, etcd, test.expectErr)
		})
	}
}

// TestValidateSpecEtcdVolumeMountsCrossFieldReference validates that all names in
// spec.etcd.volumeMounts must reference a volume declared in spec.volumes.
func TestValidateSpecEtcdVolumeMountsCrossFieldReference(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
	tests := []struct {
		name         string
		etcdName     string
		volumes      []corev1.Volume
		volumeMounts []corev1.VolumeMount
		expectErr    bool
	}{
		{
			name:     "Valid: volumeMount references declared volume",
			etcdName: "etcd-etcd-vm-ref-valid",
			volumes: []corev1.Volume{
				{Name: "my-vol", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
			volumeMounts: []corev1.VolumeMount{
				{Name: "my-vol", MountPath: "/mnt/my-vol"},
			},
			expectErr: false,
		},
		{
			name:         "Invalid: volumeMount set but spec.volumes is empty",
			etcdName:     "etcd-etcd-vm-ref-no-vols",
			volumes:      nil,
			volumeMounts: []corev1.VolumeMount{{Name: "ghost-vol", MountPath: "/mnt/ghost"}},
			expectErr:    true,
		},
		{
			name:     "Invalid: volumeMount references undeclared volume",
			etcdName: "etcd-etcd-vm-ref-invalid",
			volumes: []corev1.Volume{
				{Name: "my-vol", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
			volumeMounts: []corev1.VolumeMount{
				{Name: "other-vol", MountPath: "/mnt/other"},
			},
			expectErr: true,
		},
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(1).Build()
			etcd.Spec.Volumes = test.volumes
			etcd.Spec.Etcd.VolumeMounts = test.volumeMounts
			validateEtcdCreation(g, etcd, test.expectErr)
		})
	}
}

// TestValidateSpecBackupVolumeMountsCrossFieldReference validates that all names in
// spec.backup.volumeMounts must reference a volume declared in spec.volumes.
func TestValidateSpecBackupVolumeMountsCrossFieldReference(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
	tests := []struct {
		name         string
		etcdName     string
		volumes      []corev1.Volume
		volumeMounts []corev1.VolumeMount
		expectErr    bool
	}{
		{
			name:     "Valid: volumeMount references declared volume",
			etcdName: "etcd-backup-vm-ref-valid",
			volumes: []corev1.Volume{
				{Name: "my-vol", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
			volumeMounts: []corev1.VolumeMount{
				{Name: "my-vol", MountPath: "/mnt/my-vol"},
			},
			expectErr: false,
		},
		{
			name:         "Invalid: volumeMount set but spec.volumes is empty",
			etcdName:     "etcd-backup-vm-ref-no-vols",
			volumes:      nil,
			volumeMounts: []corev1.VolumeMount{{Name: "ghost-vol", MountPath: "/mnt/ghost"}},
			expectErr:    true,
		},
		{
			name:     "Invalid: volumeMount references undeclared volume",
			etcdName: "etcd-backup-vm-ref-invalid",
			volumes: []corev1.Volume{
				{Name: "my-vol", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
			volumeMounts: []corev1.VolumeMount{
				{Name: "other-vol", MountPath: "/mnt/other"},
			},
			expectErr: true,
		},
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(1).Build()
			etcd.Spec.Volumes = test.volumes
			etcd.Spec.Backup.VolumeMounts = test.volumeMounts
			validateEtcdCreation(g, etcd, test.expectErr)
		})
	}
}
