// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package controllers

import (
	"context"
	"sync"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/controllers/secret"
	"github.com/gardener/etcd-druid/test/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	mgrCtx     context.Context
	mgrCancel  context.CancelFunc
	cfg        *rest.Config
	k8sClient  client.Client
	testEnv    *envtest.Environment
	mgr        manager.Manager
	mgrStopped *sync.WaitGroup

	activeDeadlineDuration time.Duration

	revertFunc func()

	testLog = ctrl.Log.WithName("test")
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(
		t,
		"Controller Suite",
	)
}

var _ = BeforeSuite(func() {
	var err error

	ctrl.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	testLog.Info("Setting up test environment")
	testEnv, err = utils.SetupTestEnvironment()
	Expect(err).ToNot(HaveOccurred())

	err = druidv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	// +kubebuilder:scaffold:scheme

	k8sClient, err := client.New(testEnv.Config, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	revertFunc = utils.SwitchDirectory("../..")

	mgr, err := utils.GetManager(testEnv.Config)
	Expect(err).NotTo(HaveOccurred())

	err = addReconcilersToManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	mgrCtx, mgrCancel = context.WithCancel(context.Background())
	mgrStopped = utils.StartManager(mgrCtx, mgr)
})

var _ = AfterSuite(func() {
	utils.StopManager(mgrCancel, mgrStopped, testEnv, revertFunc)
})

func addReconcilersToManager(mgr manager.Manager) error {
	// er, err := NewEtcdReconcilerWithImageVector(mgr, false)
	// if err != nil {
	// 	return err
	// }
	// if err = er.SetupWithManager(mgr, 5, true); err != nil {
	// 	return err
	// }

	// custodian := NewEtcdCustodian(mgr, config.CustodianControllerConfig{
	// 	EtcdMember: config.EtcdMemberConfig{
	// 		EtcdMemberNotReadyThreshold: 1 * time.Minute,
	// 	},
	// })
	// if err = custodian.SetupWithManager(mgrCtx, mgr, 5, true); err != nil {
	// 	return err
	// }

	// activeDeadlineDuration, err = time.ParseDuration("2m")
	// Expect(err).NotTo(HaveOccurred())

	// lc, err := NewCompactionLeaseControllerWithImageVector(mgr, config.CompactionControllerConfig{
	// 	EnableBackupCompaction: true,
	// 	EventsThreshold:        1000000,
	// 	ActiveDeadlineDuration: activeDeadlineDuration,
	// })
	// if err != nil {
	// 	return err
	// }
	// if err = lc.SetupWithManager(mgr, 1); err != nil {
	// 	return err
	// }

	// etcdCopyBackupsTaskReconciler, err := NewEtcdCopyBackupsTaskReconcilerWithImageVector(mgr)
	// if err != nil {
	// 	return err
	// }
	// if err = etcdCopyBackupsTaskReconciler.SetupWithManager(mgr, 5); err != nil {
	// 	return err
	// }

	secretReconciler := secret.NewReconciler(mgr, &secret.Config{
		Workers: 5,
	})
	return secretReconciler.AddToManager(mgr)
}
