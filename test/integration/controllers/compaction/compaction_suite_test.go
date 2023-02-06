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
	"path/filepath"
	"testing"
	"time"

	"github.com/gardener/etcd-druid/controllers/compaction"
	"github.com/gardener/etcd-druid/controllers/utils"
	"github.com/gardener/etcd-druid/test/integration/setup"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	intTestEnv *setup.IntegrationTestEnv
)

func TestCompactionController(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(
		t,
		"Compaction Controller Suite",
	)
}

var _ = BeforeSuite(func() {
	crdPaths := getCrdPaths()
	imageVector := createImageVector()

	intTestEnv = setup.NewIntegrationTestEnv("compaction-int-tests", crdPaths)
	intTestEnv.RegisterReconcilers(func(mgr manager.Manager) {
		reconciler := compaction.NewReconcilerWithImageVector(mgr, &compaction.Config{
			EnableBackupCompaction: true,
			Workers:                5,
			EventsThreshold:        100,
			ActiveDeadlineDuration: 2 * time.Minute,
		}, imageVector)
		Expect(reconciler.AddToManager(mgr)).To(Succeed())
	}).StartManager(1 * time.Minute)
})

var _ = AfterSuite(func() {
	intTestEnv.Close()
})

func getCrdPaths() []string {
	return []string{filepath.Join("..", "..", "..", "..", "config", "crd", "bases")}
}

func createImageVector() imagevector.ImageVector {
	chartsPath := filepath.Join("..", "..", "..", "..", utils.GetDefaultImageYAMLPath())
	imageVector, err := imagevector.ReadGlobalImageVectorWithEnvOverride(chartsPath)
	Expect(err).To(BeNil())
	return imageVector
}
