// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Testing validations of etcd.spec.sharedConfig fields.

package etcd

import (
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/test/utils"
)

// validates whether the value passed to the etcd.spec.sharedConfig.autoCompactionMode is either set as "periodic" or "revision"
func TestValidateSpecSharedConfigAutoCompactionMode(t *testing.T) {
	tests := []struct {
		name               string
		etcdName           string
		autoCompactionMode string
		expectErr          bool
	}{
		{
			name:               "Valid autoCompactionMode #1: periodic",
			etcdName:           "etcd-valid-1",
			autoCompactionMode: "periodic",
			expectErr:          false,
		},
		{
			name:               "Valid autoCompactionMode #1: revision",
			etcdName:           "etcd-valid-2",
			autoCompactionMode: "revision",
			expectErr:          false,
		},
		{
			name:               "Invalid autoCompactionMode #1: invalid value",
			etcdName:           "etcd-invalid-1",
			autoCompactionMode: "dummy",
			expectErr:          true,
		},
		{
			name:               "Invalid autoCompactionMode #2: empty string",
			etcdName:           "etcd-valid-1",
			autoCompactionMode: "",
			expectErr:          true,
		},
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.Common = druidv1alpha1.SharedConfig{}
			etcd.Spec.Common.AutoCompactionMode = (*druidv1alpha1.CompactionMode)(&test.autoCompactionMode)

			validateEtcdCreation(g, etcd, test.expectErr)
		})
	}
}
