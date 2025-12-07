// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdcopybackupstask

import (
	"fmt"
	"os"
	"testing"

	"github.com/gardener/etcd-druid/test/it/assets"
	"github.com/gardener/etcd-druid/test/it/setup"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func TestMain(m *testing.M) {
	var (
		itTestEnvCloser setup.DruidTestEnvCloser
		err             error
	)

	k8sVersion, err := assets.GetK8sVersionFromEnv()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to get the kubernetes version: %v\n", err)
		os.Exit(1)
	}

	etcdCopyBackupsTaskCrd, err := assets.GetEtcdCopyBackupsTaskCrd(k8sVersion)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to get EtcdCopyBackupsTask CRD: %v\n", err)
		os.Exit(1)
	}

	itTestEnv, itTestEnvCloser, err = setup.NewDruidTestEnvironment("etcdcopybackuptask-validation", []*apiextensionsv1.CustomResourceDefinition{etcdCopyBackupsTaskCrd})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to create integration test environment: %v\n", err)
		os.Exit(1)
	}

	// os.Exit() does not respect defer statements
	exitCode := m.Run()
	itTestEnvCloser()
	os.Exit(exitCode)
}
