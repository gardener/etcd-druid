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

package compaction

import (
	"testing"
	"time"

	"github.com/gardener/etcd-druid/test/integration/controllers/assets"

	"github.com/gardener/etcd-druid/controllers/compaction"
	"github.com/gardener/etcd-druid/test/integration/setup"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	intTestEnv *setup.IntegrationTestEnv
	k8sClient  client.Client
	namespace  string
)

const (
	testNamespacePrefix = "compaction-"
)

func TestCompactionController(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(
		t,
		"Compaction Controller Suite",
	)
}

var _ = BeforeSuite(func() {
	crdPaths := []string{assets.GetEtcdCrdPath()}
	imageVector := assets.CreateImageVector()

	intTestEnv = setup.NewIntegrationTestEnv(testNamespacePrefix, "compaction-int-tests", crdPaths)
	intTestEnv.RegisterReconcilers(func(mgr manager.Manager) {
		reconciler := compaction.NewReconcilerWithImageVector(mgr, &compaction.Config{
			EnableBackupCompaction:    true,
			Workers:                   5,
			EventsThreshold:           100,
			ActiveDeadlineDuration:    2 * time.Minute,
			MetricsScrapeWaitDuration: 60 * time.Second,
		}, imageVector)
		Expect(reconciler.RegisterWithManager(mgr)).To(Succeed())
	}).StartManager()
	k8sClient = intTestEnv.K8sClient
	namespace = intTestEnv.TestNs.Name
})
