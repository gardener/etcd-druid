// Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package etcdcopybackupstask

import (
	"testing"

	"github.com/gardener/etcd-druid/controllers/etcdcopybackupstask"
	"github.com/gardener/etcd-druid/test/integration/controllers/assets"
	"github.com/gardener/etcd-druid/test/integration/setup"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	k8sClient     client.Client
	intTestEnv    *setup.IntegrationTestEnv
	imageVector   imagevector.ImageVector
	testNamespace *corev1.Namespace
)

const (
	testNamespacePrefix = "etcdcopybackupstask-"
)

func TestEtcdCopyBackupsTaskController(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(
		t,
		"EtcdCopyBackupsTask Controller Suite",
	)
}

var _ = BeforeSuite(func() {
	ctrl.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	crdPaths := []string{assets.GetEtcdCopyBackupsTaskCrdPath()}
	imageVector = assets.CreateImageVector()
	intTestEnv = setup.NewIntegrationTestEnv(testNamespacePrefix, "etcdcopybackupstask-int-tests", crdPaths)
	intTestEnv.RegisterReconcilers(func(mgr manager.Manager) {
		chartPath := assets.GetEtcdCopyBackupsBaseChartPath()
		reconciler, err := etcdcopybackupstask.NewReconcilerWithImageVector(mgr, &etcdcopybackupstask.Config{
			Workers: 5,
		}, imageVector, chartPath)
		Expect(err).To(BeNil())
		Expect(reconciler.AddToManager(mgr)).To(Succeed())
	}).StartManager()
	k8sClient = intTestEnv.K8sClient
	testNamespace = intTestEnv.TestNs
})
